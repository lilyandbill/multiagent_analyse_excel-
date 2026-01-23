package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 配置结构体
type Config struct {
	ARK     ARKConfig     `yaml:"ark,omitempty"`
	OpenAI  OpenAIConfig  `yaml:"openai,omitempty"`
	Excel   ExcelConfig   `yaml:"excel"`
	Server  ServerConfig  `yaml:"server"`
	Log     LogConfig     `yaml:"log"`
}

// ARKConfig ARK 配置
type ARKConfig struct {
	Model    string `yaml:"model,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
	BaseURL  string `yaml:"base_url,omitempty"`
	Region   string `yaml:"region,omitempty"`
}

// OpenAIConfig OpenAI 配置
type OpenAIConfig struct {
	Model    string `yaml:"model,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
	BaseURL  string `yaml:"base_url,omitempty"`
	ByAzure  bool   `yaml:"by_azure,omitempty"`
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
	// ARK 配置
	if cfg.ARK.Model == "" {
		cfg.ARK.Model = os.Getenv("ARK_MODEL")
	}
	if cfg.ARK.APIKey == "" {
		cfg.ARK.APIKey = os.Getenv("ARK_API_KEY")
	}
	if cfg.ARK.BaseURL == "" {
		cfg.ARK.BaseURL = os.Getenv("ARK_BASE_URL")
	}
	if cfg.ARK.Region == "" {
		cfg.ARK.Region = os.Getenv("ARK_REGION")
	}

	// OpenAI 配置
	if cfg.OpenAI.Model == "" {
		cfg.OpenAI.Model = os.Getenv("OPENAI_MODEL")
	}
	if cfg.OpenAI.APIKey == "" {
		cfg.OpenAI.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if cfg.OpenAI.BaseURL == "" {
		cfg.OpenAI.BaseURL = os.Getenv("OPENAI_BASE_URL")
	}
	if !cfg.OpenAI.ByAzure {
		cfg.OpenAI.ByAzure = os.Getenv("OPENAI_BY_AZURE") == "true"
	}

	return cfg
}
