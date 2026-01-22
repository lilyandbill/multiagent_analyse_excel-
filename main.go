package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"excel-agent/api/router"
	"excel-agent/config"
	"excel-agent/logger"
	"excel-agent/service"

	"go.uber.org/zap"
)

func main() {
	// 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		panic("加载配置失败: " + err.Error())
	}

	// 初始化日志
	if err := logger.Init(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		OutputPath: cfg.Log.OutputPath,
		Filename:   cfg.Log.Filename,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
	}); err != nil {
		panic("初始化日志失败: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("配置加载完成",
		zap.String("host", cfg.Server.Host),
		zap.Int("port", cfg.Server.Port),
		zap.String("log_level", cfg.Log.Level),
	)

	// 初始化服务
	excelService, err := service.NewExcelService(cfg)
	if err != nil {
		logger.Fatal("初始化 Excel 服务失败", zap.Error(err))
	}
	defer excelService.Stop()

	// 初始化路由
	r := router.NewRouter(excelService)
	r.SetupRoutes()

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	logger.Info("应用启动成功", zap.String("addr", addr))

	// 优雅关机
	go func() {
		if err := r.Run(addr); err != nil {
			logger.Error("服务器运行错误", zap.Error(err))
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("正在关闭服务器...")

	// 设置最大等待时间，然后退出
	_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Info("服务器已关闭")
}
