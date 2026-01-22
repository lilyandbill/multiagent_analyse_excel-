package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 配置结构体
type Config struct {
	LLM     LLMConfig     `yaml:"llm"`
	Excel   ExcelConfig   `yaml:"excel"`
	Server  ServerConfig  `yaml:"server"`
	Log     LogConfig     `yaml:"log"`
}

// LLMConfig LLM 配置
type LLMConfig struct {
	Model    string  `yaml:"model"`
	APIKey   string  `yaml:"api_key"`
	BaseURL  string  `yaml:"base_url,omitempty"`
	Temp     float32 `yaml:"temperature,omitempty"`
}

// ExcelConfig Excel 配置
type ExcelConfig struct {
	Dir      string  `yaml:"dir"`
	MaxRows  int     `yaml:"max_rows,omitempty"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host    string  `yaml:"host,omitempty"`
	Port    int     `yaml:"port,omitempty"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string `yaml:"level,omitempty"`       // debug, info, warn, error
	Format     string `yaml:"format,omitempty"`      // json, console
	OutputPath string `yaml:"output_path,omitempty"` // 日志输出路径
	Filename   string `yaml:"filename,omitempty"`    // 日志文件名
	MaxSize    int    `yaml:"max_size,omitempty"`    // 单个日志文件最大尺寸 (MB)
	MaxBackups int    `yaml:"max_backups,omitempty"` // 保留的旧日志文件数量
	MaxAge     int    `yaml:"max_age,omitempty"`     // 旧日志文件保留天数
}

// LoadConfig 加载配置文件
func LoadConfig(path ...string) (*Config, error) {
	configPath := "config.yaml"
	if len(path) > 0 {
		configPath = path[0]
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// 如果配置文件不存在，返回默认配置
		return getDefaultConfig(), nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 填充环境变量
	 cfg = fillEnvVars(cfg)

	return &cfg, nil
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() *Config {
	return &Config{
		LLM: LLMConfig{
			Model:  "gpt-3.5-turbo",
			APIKey: "",
			Temp:   0.7,
		},
		Excel: ExcelConfig{
			Dir:     "./excel",
			MaxRows: 10000,
		},
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Log: LogConfig{
			Level:      "info",
			Format:     "json",
			OutputPath: "./logs",
			Filename:   "app.log",
			MaxSize:    100,
			MaxBackups: 7,
			MaxAge:     7,
		},
	}
}

// fillEnvVars 填充环境变量
func fillEnvVars(cfg Config) Config {
	if cfg.LLM.APIKey == "" {
		cfg.LLM.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = os.Getenv("OPENAI_BASE_URL")
	}
	return cfg
}
