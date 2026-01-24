package handler

import (
	"excel-agent/logger"
	"excel-agent/service"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ExcelHandler Excel 处理 handler
type ExcelHandler struct {
	excelService *service.ExcelService
}

// NewExcelHandler 创建 handler
func NewExcelHandler(excelService *service.ExcelService) *ExcelHandler {
	return &ExcelHandler{
		excelService: excelService,
	}
}

// CommonResponse 通用响应结构
type CommonResponse struct {
	Success bool        `json:"success"`
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	TraceID string      `json:"trace_id,omitempty"`
	Time    int64       `json:"time"`
}

// ProcessExcelRequest 处理 Excel 请求
type ProcessExcelRequest struct {
	TaskID string `json:"task_id" binding:"required"`
	Prompt string `json:"prompt"`
}

// ListTasksRequest 任务列表请求参数
type ListTasksRequest struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	Status   string `form:"status"`
}

// newResponse 创建统一响应
func newResponse(c *gin.Context, success bool, code int, message string, data interface{}) {
	traceID := c.GetString("trace_id")
	if traceID == "" {
		traceID = strconv.FormatInt(time.Now().UnixNano(), 36)
	}

	c.JSON(http.StatusOK, CommonResponse{
		Success: success,
		Code:    code,
		Message: message,
		Data:    data,
		TraceID: traceID,
		Time:    time.Now().Unix(),
	})
}

// UploadExcel 上传 Excel 文件
// @Summary 上传 Excel 文件
// @Description 上传一个 Excel 文件并返回任务 ID
// @Tags Excel
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Excel 文件"
// @Param prompt formData string false "处理提示"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/upload [post]
func (h *ExcelHandler) UploadExcel(c *gin.Context) {
	// 获取文件
	file, err := c.FormFile("file")
	if err != nil {
		logger.Error("获取文件失败", zap.Error(err))
		newResponse(c, false, 400, "请上传有效的 Excel 文件", nil)
		return
	}

	// 打开文件
	src, err := file.Open()
	if err != nil {
		logger.Error("打开文件失败", zap.Error(err))
		newResponse(c, false, 500, "打开文件失败", nil)
		return
	}
	defer src.Close()

	// 读取文件内容
	fileContent, err := io.ReadAll(src)
	if err != nil {
		logger.Error("读取文件内容失败", zap.Error(err))
		newResponse(c, false, 500, "读取文件内容失败", nil)
		return
	}

	// 获取提示
	prompt := c.PostForm("prompt")

	// 保存文件并创建任务
	taskID, err := h.excelService.UploadExcel(c.Request.Context(), file.Filename, fileContent, prompt)
	if err != nil {
		logger.Error("上传文件失败", zap.Error(err))
		newResponse(c, false, 500, "上传文件失败: "+err.Error(), nil)
		return
	}

	logger.Info("文件上传成功", zap.String("task_id", taskID), zap.String("filename", file.Filename))

	newResponse(c, true, 0, "文件上传成功", map[string]string{
		"task_id": taskID,
	})
}

// ProcessExcel 处理 Excel 任务（同步）
// @Summary 处理 Excel 任务（同步）
// @Description 根据任务 ID 同步处理 Excel 文件
// @Tags Excel
// @Accept json
// @Produce json
// @Param task_id body string true "任务 ID"
// @Param prompt body string false "处理提示"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/process [post]
func (h *ExcelHandler) ProcessExcel(c *gin.Context) {
	var req ProcessExcelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("请求参数错误", zap.Error(err))
		newResponse(c, false, 400, "请求参数错误", nil)
		return
	}

	// 处理任务
	result, err := h.excelService.ProcessExcel(c.Request.Context(), req.TaskID, req.Prompt)
	if err != nil {
		logger.Error("处理任务失败", zap.String("task_id", req.TaskID), zap.Error(err))
		newResponse(c, false, 500, "处理任务失败: "+err.Error(), nil)
		return
	}

	logger.Info("任务处理完成", zap.String("task_id", req.TaskID))

	newResponse(c, true, 0, "处理成功", result)
}

// ProcessExcelAsync 异步处理 Excel 任务
// @Summary 异步处理 Excel 任务
// @Description 异步处理 Excel 文件，立即返回任务 ID
// @Tags Excel
// @Accept json
// @Produce json
// @Param task_id body string true "任务 ID"
// @Param prompt body string false "处理提示"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/process/async [post]
func (h *ExcelHandler) ProcessExcelAsync(c *gin.Context) {
	var req ProcessExcelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("请求参数错误", zap.Error(err))
		newResponse(c, false, 400, "请求参数错误", nil)
		return
	}

	// 异步处理任务
	err := h.excelService.ProcessExcelAsync(c.Request.Context(), req.TaskID, req.Prompt)
	if err != nil {
		logger.Error("启动异步任务失败", zap.String("task_id", req.TaskID), zap.Error(err))
		newResponse(c, false, 500, "启动异步任务失败: "+err.Error(), nil)
		return
	}

	logger.Info("异步任务已启动", zap.String("task_id", req.TaskID))

	newResponse(c, true, 0, "任务已提交处理", map[string]string{
		"task_id": req.TaskID,
	})
}

// GetTaskStatus 获取任务状态
// @Summary 获取任务状态
// @Description 根据任务 ID 查询处理状态
// @Tags Excel
// @Produce json
// @Param task_id path string true "任务 ID"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/task/{task_id} [get]
func (h *ExcelHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		newResponse(c, false, 400, "任务 ID 不能为空", nil)
		return
	}

	status, result, err := h.excelService.GetTaskStatus(c.Request.Context(), taskID)
	if err != nil {
		logger.Error("获取任务状态失败", zap.String("task_id", taskID), zap.Error(err))
		newResponse(c, false, 500, "获取任务状态失败: "+err.Error(), nil)
		return
	}

	newResponse(c, true, 0, "success", map[string]interface{}{
		"task_id": taskID,
		"status":  status,
		"result":  result,
	})
}

// DownloadResult 下载处理结果
// @Summary 下载处理结果
// @Description 下载处理后的 Excel 文件
// @Tags Excel
// @Produce octet-stream
// @Param task_id path string true "任务 ID"
// @Success 200 {file} binary
// @Router /api/v1/excel/download/{task_id} [get]
func (h *ExcelHandler) DownloadResult(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		newResponse(c, false, 400, "任务 ID 不能为空", nil)
		return
	}

	filePath, err := h.excelService.GetResultFile(c.Request.Context(), taskID)
	if err != nil {
		logger.Error("获取结果文件失败", zap.String("task_id", taskID), zap.Error(err))
		newResponse(c, false, 500, "获取结果文件失败: "+err.Error(), nil)
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename=result_"+taskID+".xlsx")
	c.File(filePath)
}

// ListTasks 列出所有任务
// @Summary 列出所有任务
// @Description 列出所有上传的 Excel 任务，支持分页和状态筛选
// @Tags Excel
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param status query string false "状态筛选"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/tasks [get]
func (h *ExcelHandler) ListTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	statusFilter := c.Query("status")

	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	tasks, total, err := h.excelService.ListTasks(c.Request.Context(), page, pageSize, statusFilter)
	if err != nil {
		logger.Error("获取任务列表失败", zap.Error(err))
		newResponse(c, false, 500, "获取任务列表失败: "+err.Error(), nil)
		return
	}

	newResponse(c, true, 0, "success", map[string]interface{}{
		"tasks":    tasks,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// PreviewFile 预览文件内容
// @Summary 预览文件内容
// @Description 根据任务 ID 预览上传的 Excel 文件内容
// @Tags Excel
// @Produce json
// @Param task_id path string true "任务 ID"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/preview/{task_id} [get]
func (h *ExcelHandler) PreviewFile(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		newResponse(c, false, 400, "任务 ID 不能为空", nil)
		return
	}

	// 获取任务信息
	task, err := h.excelService.GetTask(c.Request.Context(), taskID)
	if err != nil {
		logger.Error("获取任务信息失败", zap.String("task_id", taskID), zap.Error(err))
		newResponse(c, false, 500, "获取任务信息失败: "+err.Error(), nil)
		return
	}

	// 获取文件预览
	previews, err := h.excelService.GetFilePreviews(c.Request.Context(), taskID)
	if err != nil {
		logger.Error("获取文件预览失败", zap.String("task_id", taskID), zap.Error(err))
		newResponse(c, false, 500, "获取文件预览失败: "+err.Error(), nil)
		return
	}

	newResponse(c, true, 0, "success", map[string]interface{}{
		"task_id":  taskID,
		"filename": task.Filename,
		"status":   task.Status,
		"prompt":   task.Prompt,
		"previews": previews,
		"created":  task.CreatedAt,
	})
}

// DeleteTask 删除任务
// @Summary 删除任务
// @Description 根据任务 ID 删除任务及其文件
// @Tags Excel
// @Produce json
// @Param task_id path string true "任务 ID"
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/task/{task_id} [delete]
func (h *ExcelHandler) DeleteTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		newResponse(c, false, 400, "任务 ID 不能为空", nil)
		return
	}

	err := h.excelService.DeleteTask(c.Request.Context(), taskID)
	if err != nil {
		logger.Error("删除任务失败", zap.String("task_id", taskID), zap.Error(err))
		newResponse(c, false, 500, "删除任务失败: "+err.Error(), nil)
		return
	}

	logger.Info("任务已删除", zap.String("task_id", taskID))
	newResponse(c, true, 0, "任务删除成功", nil)
}

// AnalyzeExcel 上传并分析 Excel 文件
// @Summary 上传并分析 Excel 文件
// @Description 上传 Excel 文件并直接启动 AI 分析，支持同步和异步模式
// @Tags Excel
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Excel 文件"
// @Param prompt formData string true "处理提示"
// @Param async formData bool false "是否异步处理" default(false)
// @Success 200 {object} CommonResponse
// @Router /api/v1/excel/analyze [post]
func (h *ExcelHandler) AnalyzeExcel(c *gin.Context) {
	// 获取文件
	file, err := c.FormFile("file")
	if err != nil {
		logger.Error("获取文件失败", zap.Error(err))
		newResponse(c, false, 400, "请上传有效的 Excel 文件", nil)
		return
	}

	// 验证文件大小
	const maxFileSize = 100 * 1024 * 1024 // 100MB
	if file.Size > maxFileSize {
		logger.Warn("文件大小超过限制", zap.Int64("size", file.Size), zap.Int64("max", maxFileSize))
		newResponse(c, false, 400, "文件大小超过限制 (最大 100MB)", nil)
		return
	}

	// 打开文件
	src, err := file.Open()
	if err != nil {
		logger.Error("打开文件失败", zap.Error(err))
		newResponse(c, false, 500, "打开文件失败", nil)
		return
	}
	defer src.Close()

	// 读取文件内容
	fileContent, err := io.ReadAll(src)
	if err != nil {
		logger.Error("读取文件内容失败", zap.Error(err))
		newResponse(c, false, 500, "读取文件内容失败", nil)
		return
	}

	// 获取提示
	prompt := c.PostForm("prompt")
	if prompt == "" {
		newResponse(c, false, 400, "请提供处理提示", nil)
		return
	}

	// 获取异步模式参数
	asyncStr := c.PostForm("async")
	async := false
	if asyncStr == "true" || asyncStr == "1" {
		async = true
	}

	// 调用 Service 层分析
	result, err := h.excelService.AnalyzeExcel(c.Request.Context(), file.Filename, fileContent, prompt, async)
	if err != nil {
		logger.Error("分析失败", zap.Error(err))
		newResponse(c, false, 500, "分析失败: "+err.Error(), nil)
		return
	}

	logger.Info("分析完成", zap.String("task_id", result.TaskID), zap.Bool("async", async))

	// 构建响应数据，添加 URL
	responseData := map[string]interface{}{
		"task_id": result.TaskID,
		"status":  result.Status,
	}

	if result.Result != nil {
		responseData["result"] = result.Result
		responseData["download_url"] = fmt.Sprintf("/api/v1/excel/download/%s", result.TaskID)
	}

	if result.Status == "processing" {
		responseData["query_url"] = fmt.Sprintf("/api/v1/excel/task/%s", result.TaskID)
	}

	newResponse(c, true, 0, "分析成功", responseData)
}
