package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 全局日志实例
var log *zap.Logger

// Config 日志配置
type Config struct {
	Level      string `yaml:"level"`       // debug, info, warn, error
	Format     string `yaml:"format"`      // json, console
	OutputPath string `yaml:"output_path"` // 日志输出路径
	Filename   string `yaml:"filename"`    // 日志文件名
	MaxSize    int    `yaml:"max_size"`    // 单个日志文件最大尺寸 (MB)
	MaxBackups int    `yaml:"max_backups"` // 保留的旧日志文件数量
	MaxAge     int    `yaml:"max_age"`     // 旧日志文件保留天数
}

// Init 初始化日志
func Init(cfg Config) error {
	// 设置默认配置
	if cfg.Level == "" {
		cfg.Level = "info"
	}
	if cfg.Format == "" {
		cfg.Format = "json"
	}
	if cfg.OutputPath == "" {
		cfg.OutputPath = "./logs"
	}
	if cfg.Filename == "" {
		cfg.Filename = "app.log"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 100 // 默认 100MB
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 7
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 7
	}

	// 创建日志目录
	if err := os.MkdirAll(cfg.OutputPath, 0755); err != nil {
		return err
	}

	// 解析日志级别
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn", "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "fatal":
		level = zapcore.FatalLevel
	default:
		level = zapcore.InfoLevel
	}

	// 配置日志编码器
	var encoderConfig zapcore.EncoderConfig
	if cfg.Format == "console" {
		// 控制台格式：人类可读
		encoderConfig = zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
	} else {
		// JSON 格式：机器可读
		encoderConfig = zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}
	}

	// 配置日志文件轮转
	var writers []zapcore.WriteSyncer
	if cfg.OutputPath != "" {
		logPath := filepath.Join(cfg.OutputPath, cfg.Filename)
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}

		// 使用 zapcore 提供的日志轮转功能
		writer := zapcore.AddSync(file)
		writers = append(writers, writer)

		// 同时输出到控制台
		writers = append(writers, zapcore.AddSync(os.Stdout))
	} else {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	// 创建核心
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig),
		zapcore.NewMultiWriteSyncer(writers...),
		level,
	)

	// 创建日志实例，添加调用者信息
	log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return nil
}

// GetLogger 获取全局日志实例
func GetLogger() *zap.Logger {
	if log == nil {
		// 返回一个默认的 NopLogger，防止空指针
		return zap.NewNop()
	}
	return log
}

// Sync 同步日志缓冲
func Sync() {
	if log != nil {
		log.Sync()
	}
}

// Debug 调试级别日志
func Debug(msg string, fields ...zap.Field) {
	GetLogger().Debug(msg, fields...)
}

// Info 信息级别日志
func Info(msg string, fields ...zap.Field) {
	GetLogger().Info(msg, fields...)
}

// Warn 警告级别日志
func Warn(msg string, fields ...zap.Field) {
	GetLogger().Warn(msg, fields...)
}

// Error 错误级别日志
func Error(msg string, fields ...zap.Field) {
	GetLogger().Error(msg, fields...)
}

// Fatal 致命级别日志
func Fatal(msg string, fields ...zap.Field) {
	GetLogger().Fatal(msg, fields...)
}

// With 创建带有附加字段的日志实例
func With(fields ...zap.Field) *zap.Logger {
	return GetLogger().With(fields...)
}
