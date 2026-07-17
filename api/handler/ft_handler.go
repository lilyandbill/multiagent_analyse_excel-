package handler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"excel-agent/executor"
	"excel-agent/ft_workflow"
	"excel-agent/logger"
	"excel-agent/workflow"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	maxUploadSize = 100 * 1024 * 1024 // 100 MB
	uploadDir     = "./excel/ft_uploads"
)

// FTHandler manages the resumable FT analysis API.
type FTHandler struct {
	runStore *ft_workflow.RunStore
}

// NewFTHandler creates a new FT handler.
func NewFTHandler() *FTHandler {
	return &FTHandler{
		runStore: ft_workflow.NewRunStore(),
	}
}

// ── Request/Response Types ──────────────────────────────────────────────

type ftAnalyzeResponse struct {
	RunID       string            `json:"run_id"`
	Status      string            `json:"status"`
	Skill       string            `json:"skill"`
	DataCatalog *ftCatalogSummary `json:"data_catalog"`
	Plan        *ftPlanSummary    `json:"plan"`
}

type ftCatalogSummary struct {
	Tables   []ftTableSummary `json:"tables"`
	Warnings []string         `json:"warnings,omitempty"`
}

type ftTableSummary struct {
	FileName  string   `json:"file_name"`
	SheetName string   `json:"sheet_name"`
	RowCount  int      `json:"row_count"`
	Columns   []string `json:"columns"`
}

type ftPlanSummary struct {
	Steps []string `json:"steps"`
}

type ftConfirmRequest struct {
	RunID     string `json:"run_id" binding:"required"`
	Confirmed bool   `json:"confirmed"`
}

type ftConfirmResponse struct {
	RunID        string                          `json:"run_id"`
	Status       string                          `json:"status"`
	Yield        *ft_workflow.YieldResult        `json:"yield,omitempty"`
	Verification *ft_workflow.VerificationResult `json:"verification,omitempty"`
	ReportPath   string                          `json:"report_path,omitempty"`
	Artifacts    []string                        `json:"artifacts,omitempty"`
}

type ftStatusResponse struct {
	RunID        string         `json:"run_id"`
	Status       string         `json:"status"`
	Skill        string         `json:"skill,omitempty"`
	Plan         string         `json:"plan,omitempty"`
	Budget       map[string]any `json:"budget,omitempty"`
	Verification string         `json:"verification,omitempty"`
	Error        string         `json:"error,omitempty"`
	CreatedAt    string         `json:"created_at,omitempty"`
	UpdatedAt    string         `json:"updated_at,omitempty"`
}

// ── Upload and Plan ─────────────────────────────────────────────────────

// AnalyzeFT uploads an FT Excel file, builds the catalog, generates a plan,
// and stops at WAITING_CONFIRMATION.
func (h *FTHandler) AnalyzeFT(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		newResponse(c, false, 400, "请上传有效的 Excel 文件", nil)
		return
	}

	// Validate size.
	if file.Size > maxUploadSize {
		newResponse(c, false, 400, "文件大小超过限制 (最大 100MB)", nil)
		return
	}

	// Validate extension.
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		newResponse(c, false, 400, "不支持的文件格式，请上传 .xlsx .xls 或 .csv 文件", nil)
		return
	}

	// Sanitize filename.
	safeName := sanitizeFilename(file.Filename)

	// Open file.
	src, err := file.Open()
	if err != nil {
		logger.Error("打开文件失败", zap.Error(err))
		newResponse(c, false, 500, "打开文件失败", nil)
		return
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		logger.Error("读取文件失败", zap.Error(err))
		newResponse(c, false, 500, "读取文件失败", nil)
		return
	}

	// Create isolated workspace.
	runID := "run_" + uuid.New().String()[:8]
	workDir := filepath.Join(uploadDir, runID)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		logger.Error("创建工作目录失败", zap.Error(err))
		newResponse(c, false, 500, "创建运行空间失败", nil)
		return
	}

	excelPath := filepath.Join(workDir, safeName)
	if err := os.WriteFile(excelPath, content, 0644); err != nil {
		logger.Error("保存文件失败", zap.Error(err))
		newResponse(c, false, 500, "保存文件失败", nil)
		return
	}

	task := c.PostForm("task")
	if task == "" {
		task = "分析 FT Yield"
	}

	// Build orchestrator and execute up to WAITING_APPROVAL.
	orch := ft_workflow.NewOrchestrator(ft_workflow.OrchestratorConfig{
		TaskID:   runID,
		WorkDir:  workDir,
		Task:     task,
		Executor: executor.FakeExecutor{},
	})

	if err := orch.Ingest(c.Request.Context()); err != nil {
		logger.Error("数据采集失败", zap.Error(err))
		newResponse(c, false, 500, "数据采集失败: "+err.Error(), nil)
		return
	}

	if err := orch.Plan(c.Request.Context(), task); err != nil {
		logger.Error("生成计划失败", zap.Error(err))
		newResponse(c, false, 500, "生成计划失败: "+err.Error(), nil)
		return
	}

	if err := orch.WaitForApproval(c.Request.Context()); err != nil {
		logger.Error("等待确认失败", zap.Error(err))
		newResponse(c, false, 500, "状态转换失败", nil)
		return
	}

	// Save run state.
	state := orch.MarshalState(task)
	if err := h.runStore.Save(runID, state); err != nil {
		logger.Error("保存运行状态失败", zap.Error(err))
		newResponse(c, false, 500, "保存运行状态失败", nil)
		return
	}

	// Build response.
	catalog := buildCatalogSummary(orch.Catalog)
	plan := buildPlanSummary(orch.PlanText)

	newResponse(c, true, 0, "计划已生成，等待用户确认", ftAnalyzeResponse{
		RunID:       runID,
		Status:      "WAITING_CONFIRMATION",
		Skill:       "ft_yield_analysis",
		DataCatalog: catalog,
		Plan:        plan,
	})
}

// ── Confirmation ────────────────────────────────────────────────────────

// ConfirmFT handles user confirmation to proceed or cancel.
func (h *FTHandler) ConfirmFT(c *gin.Context) {
	var req ftConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		newResponse(c, false, 400, "请求参数无效: run_id 必填", nil)
		return
	}

	// Handle cancellation.
	if !req.Confirmed {
		state, err := h.runStore.Load(req.RunID)
		if err != nil {
			newResponse(c, false, 404, "运行未找到: "+req.RunID, nil)
			return
		}
		if state.State != workflow.StateWaitingApproval {
			newResponse(c, false, 409, fmt.Sprintf("无法取消: 当前状态为 %s", state.State), nil)
			return
		}
		state.State = workflow.StateFailed
		h.runStore.Save(req.RunID, state)
		newResponse(c, true, 0, "运行已取消", ftConfirmResponse{
			RunID:  req.RunID,
			Status: "FAILED",
		})
		return
	}

	// Lock for execution — prevents duplicate confirmation.
	state, err := h.runStore.LoadAndLock(req.RunID)
	if err != nil {
		newResponse(c, false, 409, err.Error(), nil)
		return
	}

	// Restore orchestrator.
	orch := ft_workflow.RestoreOrchestrator(state, executor.FakeExecutor{})

	// Find the Excel file.
	excelPath, sheetName, err := findExcelInDir(orch.Workflow.WorkDir)
	if err != nil {
		h.failRun(c, req.RunID, state, "查找文件失败: "+err.Error())
		return
	}

	// Determine yield params from catalog.
	params := ft_workflow.YieldParams{
		PassColumn: "TEST_RESULT",
		PassValue:  "PASS",
		GroupBy:    "LOT_ID",
	}
	if state.Catalog != nil && len(state.Catalog.Tables) > 0 {
		// Use first table's sheet name.
		sheetName = state.Catalog.Tables[0].SheetName
	}

	// If restoring from WAITING_APPROVAL, confirm first.
	// If state was already advanced (e.g. EXECUTING), skip Confirm.
	if orch.CurrentState() == workflow.StateWaitingApproval {
		if err := orch.Confirm(c.Request.Context()); err != nil {
			h.failRun(c, req.RunID, state, "确认失败: "+err.Error())
			return
		}
	}

	if err := orch.ExecuteYield(c.Request.Context(), excelPath, sheetName, params); err != nil {
		h.failRun(c, req.RunID, state, "计算 Yield 失败: "+err.Error())
		return
	}

	var expectedCols []string
	if state.Catalog != nil && len(state.Catalog.Tables) > 0 {
		expectedCols = state.Catalog.ColumnNames(0)
	}

	if err := orch.Verify(c.Request.Context(), expectedCols); err != nil {
		// Verification failure — persist diagnostics, transition to FAILED.
		state = orch.MarshalState(state.Task)
		h.runStore.Save(req.RunID, state)
		newResponse(c, false, 422, "验证失败: "+err.Error(), ftConfirmResponse{
			RunID:        req.RunID,
			Status:       "FAILED",
			Verification: orch.Verification,
		})
		return
	}

	if err := orch.GenerateReport(c.Request.Context(), state.Task); err != nil {
		h.failRun(c, req.RunID, state, "生成报告失败: "+err.Error())
		return
	}

	if err := orch.Complete(c.Request.Context()); err != nil {
		h.failRun(c, req.RunID, state, "状态转换失败: "+err.Error())
		return
	}

	// Save final state.
	finalState := orch.MarshalState(state.Task)
	h.runStore.Save(req.RunID, finalState)

	newResponse(c, true, 0, "分析完成", ftConfirmResponse{
		RunID:        req.RunID,
		Status:       "DONE",
		Yield:        orch.YieldResult,
		Verification: orch.Verification,
		ReportPath:   orch.ReportPath,
	})
}

func (h *FTHandler) failRun(c *gin.Context, runID string, state ft_workflow.RunState, msg string) {
	logger.Error("FT 运行失败", zap.String("run_id", runID), zap.String("error", msg))
	state.State = workflow.StateFailed
	h.runStore.Save(runID, state)
	newResponse(c, false, 500, msg, nil)
}

// ── Status ──────────────────────────────────────────────────────────────

// GetFTStatus returns the current state of a run.
func (h *FTHandler) GetFTStatus(c *gin.Context) {
	runID := c.Param("run_id")
	if runID == "" {
		newResponse(c, false, 400, "run_id 不能为空", nil)
		return
	}

	state, err := h.runStore.Load(runID)
	if err != nil {
		newResponse(c, false, 404, "运行未找到", nil)
		return
	}

	budget := map[string]any{
		"llm_calls":  state.Usage.LLMCalls,
		"tool_calls": state.Usage.ToolCalls,
		"iterations": state.Usage.Iterations,
	}

	var verifyStatus string
	if state.Verification != nil {
		verifyStatus = string(state.Verification.Status)
	}

	newResponse(c, true, 0, "success", ftStatusResponse{
		RunID:        runID,
		Status:       string(state.State),
		Skill:        "ft_yield_analysis",
		Plan:         state.PlanText,
		Budget:       budget,
		Verification: verifyStatus,
		CreatedAt:    state.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    state.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// ── Report ───────────────────────────────────────────────────────────────

// GetFTReport returns the report artifact content.
func (h *FTHandler) GetFTReport(c *gin.Context) {
	runID := c.Param("run_id")
	if runID == "" {
		newResponse(c, false, 400, "run_id 不能为空", nil)
		return
	}

	state, err := h.runStore.Load(runID)
	if err != nil {
		newResponse(c, false, 404, "运行未找到", nil)
		return
	}

	if state.State != workflow.StateDone {
		newResponse(c, false, 409, fmt.Sprintf("报告尚未生成，当前状态: %s", state.State), nil)
		return
	}

	if state.Verification == nil || !state.Verification.Passed {
		newResponse(c, false, 422, "验证未通过，无法提供报告", nil)
		return
	}

	// Return report path and summary — actual content is in the artifact store.
	newResponse(c, true, 0, "success", map[string]string{
		"run_id":      runID,
		"report_path": state.ReportPath,
		"plan":        state.PlanText,
	})
}

// ── Helpers ─────────────────────────────────────────────────────────────

func buildCatalogSummary(catalog *ft_workflow.DataCatalog) *ftCatalogSummary {
	if catalog == nil {
		return &ftCatalogSummary{}
	}
	cs := &ftCatalogSummary{}
	for _, t := range catalog.Tables {
		cols := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			cols[i] = c.Name
		}
		cs.Tables = append(cs.Tables, ftTableSummary{
			FileName:  t.FileName,
			SheetName: t.SheetName,
			RowCount:  t.RowCount,
			Columns:   cols,
		})
	}
	return cs
}

func buildPlanSummary(planText string) *ftPlanSummary {
	steps := strings.Split(strings.TrimSpace(planText), "\n")
	var filtered []string
	for _, s := range steps {
		s = strings.TrimSpace(s)
		if s != "" {
			filtered = append(filtered, s)
		}
	}
	return &ftPlanSummary{Steps: filtered}
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." {
		name = "upload.xlsx"
	}
	return name
}

func findExcelInDir(dir string) (string, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", fmt.Errorf("read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".xlsx" || ext == ".xls" || ext == ".csv" {
			return filepath.Join(dir, e.Name()), "Sheet1", nil
		}
	}
	return "", "", fmt.Errorf("no Excel file found in %s", dir)
}
