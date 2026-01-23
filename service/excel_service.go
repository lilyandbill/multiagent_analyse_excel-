package service

import (
	"context"
	"encoding/json"
	"excel-agent/agents/executor"
	"excel-agent/agents/planner"
	"excel-agent/agents/replanner"
	"excel-agent/agents/report"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"excel-agent/config"
	"excel-agent/generic"
	"excel-agent/logger"
	"excel-agent/params"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusProcessing TaskStatus = "processing"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// TaskStatusMap 状态映射
var TaskStatusMap = map[TaskStatus]string{
	TaskStatusPending:    "pending",
	TaskStatusProcessing: "processing",
	TaskStatusCompleted:  "completed",
	TaskStatusFailed:     "failed",
}

// ExcelTask Excel 任务
type ExcelTask struct {
	TaskID     string      `json:"task_id"`
	Filename   string      `json:"filename"`
	Prompt     string      `json:"prompt"`
	Status     TaskStatus  `json:"status"`
	Result     interface{} `json:"result,omitempty"`
	ResultFile string      `json:"result_file,omitempty"`
	WorkDir    string      `json:"work_dir"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// ExcelService Excel 服务
type ExcelService struct {
	cfg           *config.Config
	tasks         map[string]*ExcelTask
	taskMu        sync.RWMutex
	uploadDir     string
	resultDir     string
	agent         adk.Agent
	taskExpiry    time.Duration // 任务过期时间
	cleanupTicker *time.Ticker  // 清理定时器
	stopChan      chan struct{}
}

// ToJSONString 将对象转换为 JSON 字符串
func ToJSONString(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// NewExcelService 创建服务
func NewExcelService(cfg *config.Config) (*ExcelService, error) {
	// 创建上传和结果目录
	uploadDir := filepath.Join(cfg.Excel.Dir, "uploads")
	resultDir := filepath.Join(cfg.Excel.Dir, "results")

	for _, dir := range []string{uploadDir, resultDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}
	ctx := context.Background()
	// 创建excel agent
	agent, err := newExcelAgent(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("创建excel agent失败, err: %w", err)
	}

	svc := &ExcelService{
		cfg:        cfg,
		tasks:      make(map[string]*ExcelTask),
		uploadDir:  uploadDir,
		resultDir:  resultDir,
		taskExpiry: 24 * time.Hour, // 默认 24 小时过期
		stopChan:   make(chan struct{}),
		agent:      agent,
	}

	// 启动过期任务清理 goroutine
	go svc.startCleanupWorker()

	return svc, nil
}

// startCleanupWorker 启动过期任务清理工作
func (s *ExcelService) startCleanupWorker() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时检查一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpiredTasks()
		case <-s.stopChan:
			return
		}
	}
}

// cleanupExpiredTasks 清理过期任务
func (s *ExcelService) cleanupExpiredTasks() {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()

	now := time.Now()
	expiredCount := 0

	for taskID, task := range s.tasks {
		if now.Sub(task.CreatedAt) > s.taskExpiry {
			// 删除任务目录
			if task.WorkDir != "" {
				os.RemoveAll(task.WorkDir)
			}
			// 删除结果文件
			resultFile := filepath.Join(s.resultDir, "result_"+taskID+".xlsx")
			os.Remove(resultFile)

			delete(s.tasks, taskID)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		logger.Info("清理过期任务", zap.Int("count", expiredCount))
	}
}

// Stop 停止服务
func (s *ExcelService) Stop() {
	close(s.stopChan)
}

// SetAgent 设置 AI Agent
func (s *ExcelService) SetAgent(agent adk.Agent) {
	s.agent = agent
}

// GetAgent 获取 AI Agent
func (s *ExcelService) GetAgent() adk.Agent {
	return s.agent
}

// SetTaskExpiry 设置任务过期时间
func (s *ExcelService) SetTaskExpiry(duration time.Duration) {
	s.taskExpiry = duration
}

// UploadExcel 上传 Excel 文件
func (s *ExcelService) UploadExcel(ctx context.Context, filename string, fileContent []byte, prompt string) (string, error) {
	taskID := uuid.New().String()
	workDir := filepath.Join(s.uploadDir, taskID)

	// 创建工作目录
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", fmt.Errorf("创建工作目录失败: %w", err)
	}

	// 保存文件
	filePath := filepath.Join(workDir, filename)
	if err := os.WriteFile(filePath, fileContent, 0644); err != nil {
		return "", fmt.Errorf("保存文件失败: %w", err)
	}

	// 预览文件
	previews, err := generic.PreviewPath(workDir)
	if err != nil {
		logger.Warn("预览文件失败", zap.Error(err))
	}

	s.taskMu.Lock()
	s.tasks[taskID] = &ExcelTask{
		TaskID:    taskID,
		Filename:  filename,
		Prompt:    prompt,
		Status:    TaskStatusPending,
		WorkDir:   workDir,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.taskMu.Unlock()

	logger.Info("文件上传成功", zap.String("task_id", taskID), zap.String("filename", filename), zap.Any("previews", previews))

	return taskID, nil
}

// ProcessExcel 处理 Excel 文件（同步方式）
func (s *ExcelService) ProcessExcel(ctx context.Context, taskID string, prompt string) (interface{}, error) {
	s.taskMu.RLock()
	task, exists := s.tasks[taskID]
	s.taskMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}

	// 检查上下文是否取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 更新状态为处理中
	s.taskMu.Lock()
	task.Status = TaskStatusProcessing
	task.UpdatedAt = time.Now()
	s.taskMu.Unlock()

	// 如果没有传入 prompt，使用任务创建时的 prompt
	if prompt == "" {
		prompt = task.Prompt
	}

	// 预览文件
	previews, err := generic.PreviewPath(task.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("预览文件失败: %w", err)
	}
	
	logger.Info("开始处理任务", zap.String("task_id", taskID), zap.String("prompt", prompt))

	// 使用 eino agent 处理
	if s.agent != nil {
		result, err := s.runAgent(ctx, taskID, prompt, task.WorkDir, previews)
		s.taskMu.Lock()
		if err != nil {
			task.Status = TaskStatusFailed
			task.UpdatedAt = time.Now()
			s.taskMu.Unlock()
			return nil, err
		}

		task.Status = TaskStatusCompleted
		task.Result = result
		task.UpdatedAt = time.Now()
		s.taskMu.Unlock()

		return result, nil
	}

	// 如果没有 agent，使用模拟处理
	result := s.mockProcess(taskID, prompt, previews)
	s.taskMu.Lock()
	task.Status = TaskStatusCompleted
	task.Result = result
	task.UpdatedAt = time.Now()
	s.taskMu.Unlock()

	return result, nil
}

// ProcessExcelAsync 异步处理 Excel 文件
func (s *ExcelService) ProcessExcelAsync(ctx context.Context, taskID string, prompt string) error {
	s.taskMu.RLock()
	_, exists := s.tasks[taskID]
	s.taskMu.RUnlock()

	if !exists {
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	// 使用带超时的上下文
	asyncCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	go func() {
		_, err := s.ProcessExcel(asyncCtx, taskID, prompt)
		if err != nil {
			logger.Error("异步处理失败", zap.String("task_id", taskID), zap.Error(err))
		}
	}()

	return nil
}

// runAgent 运行 AI Agent 处理任务
func (s *ExcelService) runAgent(ctx context.Context, taskID, query, workDir string, previews []*generic.PreviewFile) (interface{}, error) {
	// 设置上下文参数
	ctx = params.InitContextParams(ctx)
	params.AppendContextParams(ctx, map[string]interface{}{
		params.FilePathSessionKey:            workDir,
		params.WorkDirSessionKey:             workDir,
		params.UserAllPreviewFilesSessionKey: ToJSONString(previews),
		params.TaskIDKey:                     taskID,
	})

	// 创建消息
	userMsg := schema.UserMessage(query)

	// 运行 agent
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           s.agent,
		EnableStreaming: false,
	})

	iter := runner.Run(ctx, []*schema.Message{userMsg})

	var lastMessage *schema.Message
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Output != nil && event.Output.MessageOutput != nil {
				lastMessage = event.Output.MessageOutput.Message
			}
		}
	}

	if lastMessage == nil {
		return nil, fmt.Errorf("处理未返回结果")
	}

	// 解析结果
	result := map[string]interface{}{
		"task_id":  taskID,
		"message":  lastMessage.Content,
		"role":     lastMessage.Role,
		"previews": previews,
	}

	return result, nil
}

// mockProcess 模拟处理（用于测试）
func (s *ExcelService) mockProcess(taskID, prompt string, previews []*generic.PreviewFile) map[string]interface{} {
	return map[string]interface{}{
		"task_id":   taskID,
		"message":   fmt.Sprintf("处理完成: %s", prompt),
		"sheets":    s.getSheetNames(previews),
		"processed": true,
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
	}
}

// getSheetNames 获取工作表名称列表
func (s *ExcelService) getSheetNames(previews []*generic.PreviewFile) []string {
	var sheets []string
	for _, p := range previews {
		for _, sfp := range p.SingleFilePreviews {
			sheets = append(sheets, sfp.SheetName)
		}
	}
	return sheets
}

// GetTaskStatus 获取任务状态
func (s *ExcelService) GetTaskStatus(ctx context.Context, taskID string) (TaskStatus, interface{}, error) {
	s.taskMu.RLock()
	defer s.taskMu.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return TaskStatusFailed, nil, fmt.Errorf("任务不存在: %s", taskID)
	}

	return task.Status, task.Result, nil
}

// GetTask 获取任务
func (s *ExcelService) GetTask(ctx context.Context, taskID string) (*ExcelTask, error) {
	s.taskMu.RLock()
	defer s.taskMu.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}

	return task, nil
}

// GetFilePreviews 获取文件预览
func (s *ExcelService) GetFilePreviews(ctx context.Context, taskID string) ([]*generic.PreviewFile, error) {
	s.taskMu.RLock()
	task, exists := s.tasks[taskID]
	s.taskMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", taskID)
	}

	return generic.PreviewPath(task.WorkDir)
}

// GetResultFile 获取结果文件路径
func (s *ExcelService) GetResultFile(ctx context.Context, taskID string) (string, error) {
	s.taskMu.RLock()
	_, exists := s.tasks[taskID]
	s.taskMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("任务不存在: %s", taskID)
	}

	resultFile := filepath.Join(s.resultDir, "result_"+taskID+".xlsx")
	if _, err := os.Stat(resultFile); os.IsNotExist(err) {
		// 如果结果文件不存在，创建一个空的
		if err := s.createMockResultFile(resultFile); err != nil {
			return "", err
		}
	}

	return resultFile, nil
}

// ListTasks 列出所有任务（支持分页和状态筛选，按时间倒序）
func (s *ExcelService) ListTasks(ctx context.Context, page, pageSize int, statusFilter string) ([]*ExcelTask, int, error) {
	s.taskMu.RLock()
	defer s.taskMu.RUnlock()

	var filteredTasks []*ExcelTask

	for _, task := range s.tasks {
		// 状态筛选
		if statusFilter != "" {
			if string(task.Status) != strings.ToLower(statusFilter) {
				continue
			}
		}
		filteredTasks = append(filteredTasks, task)
	}

	// 按创建时间倒序排序
	sort.Slice(filteredTasks, func(i, j int) bool {
		return filteredTasks[i].CreatedAt.After(filteredTasks[j].CreatedAt)
	})

	// 计算分页
	total := len(filteredTasks)
	start := (page - 1) * pageSize
	end := start + pageSize

	if start > total {
		return []*ExcelTask{}, total, nil
	}
	if end > total {
		end = total
	}

	return filteredTasks[start:end], total, nil
}

// DeleteTask 删除任务
func (s *ExcelService) DeleteTask(ctx context.Context, taskID string) error {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("任务不存在: %s", taskID)
	}

	// 删除任务目录
	if task.WorkDir != "" {
		os.RemoveAll(task.WorkDir)
	}

	// 删除结果文件
	resultFile := filepath.Join(s.resultDir, "result_"+taskID+".xlsx")
	os.Remove(resultFile)

	// 从 map 中删除
	delete(s.tasks, taskID)

	return nil
}

// createMockResultFile 创建模拟结果文件
func (s *ExcelService) createMockResultFile(path string) error {
	f := excelize.NewFile()
	defer f.Close()

	// 添加一些示例数据
	f.SetCellValue("Sheet1", "A1", "处理结果")
	f.SetCellValue("Sheet1", "A2", "任务完成")
	f.SetCellValue("Sheet1", "B2", time.Now().Format("2006-01-02 15:04:05"))

	return f.SaveAs(path)
}

// GetTaskCount 获取任务数量统计
func (s *ExcelService) GetTaskCount() map[TaskStatus]int {
	s.taskMu.RLock()
	defer s.taskMu.RUnlock()

	counts := make(map[TaskStatus]int)
	for _, task := range s.tasks {
		counts[task.Status]++
	}
	return counts
}

// AnalysisResult 分析结果
type AnalysisResult struct {
	TaskID      string                 `json:"task_id"`
	Status      TaskStatus             `json:"status"`
	Result      interface{}            `json:"result,omitempty"`
	DownloadURL string                 `json:"download_url,omitempty"`
	QueryURL    string                 `json:"query_url,omitempty"`
}

// AnalyzeExcel 上传并分析 Excel 文件（支持同步和异步模式）
func (s *ExcelService) AnalyzeExcel(ctx context.Context, filename string, fileContent []byte, prompt string, async bool) (*AnalysisResult, error) {
	// 1. 上传文件并创建任务
	taskID, err := s.UploadExcel(ctx, filename, fileContent, prompt)
	if err != nil {
		return nil, err
	}

	// 2. 异步模式：立即返回 task_id
	if async {
		return &AnalysisResult{
			TaskID:   taskID,
			Status:   TaskStatusProcessing,
			QueryURL: fmt.Sprintf("/api/v1/excel/task/%s", taskID),
		}, nil
	}

	// 3. 同步模式：处理并返回结果
	result, err := s.ProcessExcel(ctx, taskID, prompt)
	if err != nil {
		return nil, err
	}

	return &AnalysisResult{
		TaskID:      taskID,
		Status:      TaskStatusCompleted,
		Result:      result,
		DownloadURL: fmt.Sprintf("/api/v1/excel/download/%s", taskID),
	}, nil
}

func newExcelAgent(ctx context.Context, cfg *config.Config) (adk.Agent, error) {
	operator := &LocalOperator{}

	p, err := planner.NewPlanner(ctx, operator, cfg)
	if err != nil {
		return nil, err
	}

	e, err := executor.NewExecutor(ctx, operator)
	if err != nil {
		return nil, err
	}

	rp, err := replanner.NewReplanner(ctx, operator)
	if err != nil {
		return nil, err
	}

	planExecuteAgent, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       p,
		Executor:      e,
		Replanner:     rp,
		MaxIterations: 20,
	})
	if err != nil {
		return nil, err
	}

	reportAgent, err := report.NewReportAgent(ctx, operator)
	if err != nil {
		return nil, err
	}

	agent, err := adk.NewSequentialAgent(ctx, &adk.SequentialAgentConfig{
		Name:        "SequentialAgent",
		Description: "sequential agent",
		SubAgents: []adk.Agent{
			planExecuteAgent,
			reportAgent,
		},
	})
	if err != nil {
		return nil, err
	}

	return agent, nil
}
