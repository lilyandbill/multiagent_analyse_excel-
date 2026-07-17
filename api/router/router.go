package router

import (
	"excel-agent/api/handler"
	"excel-agent/logger"
	"excel-agent/service"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Router 路由配置
type Router struct {
	engine       *gin.Engine
	excelHandler *handler.ExcelHandler
	ftHandler    *handler.FTHandler
}

// NewRouter 创建路由
func NewRouter(excelService *service.ExcelService) *Router {
	// 设置 gin 模式为 Release（生产环境）
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()

	// 添加中间件
	engine.Use(gin.Recovery())
	engine.Use(requestIDMiddleware())
	engine.Use(loggerMiddleware())
	engine.Use(corsMiddleware())
	engine.Use(rateLimitMiddleware())

	return &Router{
		engine:       engine,
		excelHandler: handler.NewExcelHandler(excelService),
		ftHandler:    handler.NewFTHandler(),
	}
}

// SetupRoutes 配置路由
func (r *Router) SetupRoutes() {
	// 健康检查
	r.engine.GET("/health", healthCheck)

	// API v1 路由组
	v1 := r.engine.Group("/api/v1")
	{
		// Excel 相关接口
		excel := v1.Group("/excel")
		{
			// 分析接口（新）
			excel.POST("/analyze", r.excelHandler.AnalyzeExcel)

			// 任务管理
			excel.GET("/tasks", r.excelHandler.ListTasks)
			excel.GET("/task/:task_id", r.excelHandler.GetTaskStatus)
			excel.GET("/preview/:task_id", r.excelHandler.PreviewFile)
			excel.GET("/download/:task_id", r.excelHandler.DownloadResult)
			excel.DELETE("/task/:task_id", r.excelHandler.DeleteTask)
		}

		// FT Yield 分析 (V2 single-agent workflow)
		ft := v1.Group("/ft")
		{
			ft.POST("/analyze", r.ftHandler.AnalyzeFT)
			ft.POST("/confirm", r.ftHandler.ConfirmFT)
			ft.GET("/status/:run_id", r.ftHandler.GetFTStatus)
			ft.GET("/report/:run_id", r.ftHandler.GetFTReport)
		}

		// 统计接口
		v1.GET("/stats", r.getStats)
	}
}

// Run 启动服务器
func (r *Router) Run(addr string) error {
	logger.Info("启动 HTTP 服务器", zap.String("addr", addr))
	return r.engine.Run(addr)
}

// Engine 获取 gin engine（用于测试）
func (r *Router) Engine() *gin.Engine {
	return r.engine
}

// requestIDMiddleware 请求 ID 中间件
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 尝试从 header 获取，否则生成新的
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = strconv.FormatInt(time.Now().UnixNano(), 36)
		}

		c.Set("trace_id", traceID)
		c.Header("X-Trace-ID", traceID)

		c.Next()
	}
}

// loggerMiddleware 日志中间件
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		traceID := c.GetString("trace_id")

		// 处理请求
		c.Next()

		// 记录请求日志
		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Info("HTTP 请求",
			zap.String("trace_id", traceID),
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}

// corsMiddleware 跨域中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-ID")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Trace-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// rateLimitMiddleware 简单的限流中间件
// 注意：生产环境建议使用更完善的限流方案如 golang.org/x/time/rate
var requestCount = make(map[string]int)
var countMu sync.Mutex

func rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()

		countMu.Lock()
		requestCount[clientIP]++
		count := requestCount[clientIP]
		countMu.Unlock()

		// 每分钟重置计数器
		if count > 1000 { // 简单限制：每分钟最多 1000 个请求
			c.AbortWithStatusJSON(429, gin.H{
				"success": false,
				"code":    429,
				"message": "请求过于频繁，请稍后再试",
			})
			return
		}

		c.Next()
	}
}

// getStats 获取统计信息
func (r *Router) getStats(c *gin.Context) {
	// 这里可以从 service 获取统计数据
	c.JSON(200, gin.H{
		"success": true,
		"message": "统计信息",
		"data": gin.H{
			"endpoints": []string{
				"POST   /api/v1/excel/analyze",
				"GET    /api/v1/excel/task/:task_id",
				"GET    /api/v1/excel/preview/:task_id",
				"GET    /api/v1/excel/download/:task_id",
				"GET    /api/v1/excel/tasks",
				"DELETE /api/v1/excel/task/:task_id",
				"GET    /health",
			},
		},
	})
}

// healthCheck 健康检查
func healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "ok",
		"message": "服务正常运行",
		"time":    time.Now().Unix(),
	})
}
