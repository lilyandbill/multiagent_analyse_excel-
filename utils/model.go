/*
 * Copyright 2025 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package utils

import (
	"context"

	"excel-agent/config"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	arkmodel "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

type CreateChatModelOption func(o *option)

func NewChatModel(ctx context.Context, opts ...CreateChatModelOption) (cm model.ToolCallingChatModel, err error) {
	o := &option{}
	for _, opt := range opts {
		opt(o)
	}

	// 如果没有提供配置，尝试从配置文件加载
	var cfg *config.Config
	if o.Config != nil {
		cfg = o.Config
	} else {
		cfg, err = config.LoadConfig()
		if err != nil {
			return nil, err
		}
	}

	// 优先检查 ARK 配置
	if cfg.ARK.Model != "" {
		conf := &ark.ChatModelConfig{
			APIKey:      cfg.ARK.APIKey,
			BaseURL:     cfg.ARK.BaseURL,
			Region:      cfg.ARK.Region,
			Model:       cfg.ARK.Model,
			MaxTokens:   o.MaxTokens,
			Temperature: o.Temperature,
			TopP:        o.TopP,
		}
		if o.DisableThinking != nil && *o.DisableThinking {
			conf.Thinking = &arkmodel.Thinking{
				Type: arkmodel.ThinkingTypeDisabled,
			}
		}
		if o.JsonSchema != nil {
			conf.ResponseFormat = &ark.ResponseFormat{
				Type: arkmodel.ResponseFormatJSONSchema,
				JSONSchema: &arkmodel.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:        o.JsonSchema.Name,
					Description: o.JsonSchema.Description,
					Schema:      o.JsonSchema.JSONSchema,
					Strict:      o.JsonSchema.Strict,
				},
			}
		}
		cm, err = ark.NewChatModel(ctx, conf)
	} else if cfg.OpenAI.Model != "" {
		conf := &openai.ChatModelConfig{
			APIKey:      cfg.OpenAI.APIKey,
			ByAzure:     cfg.OpenAI.ByAzure,
			BaseURL:     cfg.OpenAI.BaseURL,
			Model:       cfg.OpenAI.Model,
			MaxTokens:   o.MaxTokens,
			Temperature: o.Temperature,
			TopP:        o.TopP,
		}
		if o.JsonSchema != nil {
			conf.ResponseFormat = &openai.ChatCompletionResponseFormat{
				Type:       openai.ChatCompletionResponseFormatTypeJSONSchema,
				JSONSchema: o.JsonSchema,
			}
		}
		cm, err = openai.NewChatModel(ctx, conf)
	}
	if err != nil {
		return nil, err
	}

	return cm, nil
}

type option struct {
	Config          *config.Config
	MaxTokens       *int
	Temperature     *float32
	TopP            *float32
	DisableThinking *bool
	JsonSchema      *openai.ChatCompletionResponseFormatJSONSchema
}

func WithMaxTokens(maxTokens int) CreateChatModelOption {
	return func(o *option) {
		o.MaxTokens = &maxTokens
	}
}

func WithTemperature(temp float32) CreateChatModelOption {
	return func(o *option) {
		o.Temperature = &temp
	}
}

func WithTopP(topP float32) CreateChatModelOption {
	return func(o *option) {
		o.TopP = &topP
	}
}

func WithDisableThinking(disable bool) CreateChatModelOption {
	return func(o *option) {
		o.DisableThinking = &disable
	}
}

func WithResponseFormatJsonSchema(schema *openai.ChatCompletionResponseFormatJSONSchema) CreateChatModelOption {
	return func(o *option) {
		o.JsonSchema = schema
	}
}
