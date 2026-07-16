package ft_workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"excel-agent/artifacts"
	"excel-agent/executor"
	"excel-agent/hooks"
	"excel-agent/skills"
	"excel-agent/workflow"
)

// Orchestrator is the single Main Agent for FT yield analysis.
// It coordinates the workflow state machine, deterministic tools, artifact store,
// and optionally an LLM executor for plan generation and summarization.
type Orchestrator struct {
	Workflow     *workflow.TaskContext
	Skills       *skills.Registry
	Hooks        *hooks.Pipeline
	Artifacts    *artifacts.Store
	Executor     executor.Executor
	Catalog      *DataCatalog
	PlanText     string
	YieldParams  YieldParams
	YieldResult  *YieldResult
	Verification *VerificationResult
	ReportPath   string
	artifactRefs []string
}

// OrchestratorConfig configures the orchestrator.
type OrchestratorConfig struct {
	TaskID   string
	WorkDir  string
	Task     string
	Executor executor.Executor
}

// NewOrchestrator creates a new orchestrator with all subsystems initialized.
func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	hookReg := hooks.NewRegistry()
	pipeline := hooks.NewPipeline(hookReg)

	skillReg := skills.NewRegistry()
	skill, _ := skills.NewSkill(skills.Manifest{
		Name:        skills.SkillFTYieldAnalysis,
		DisplayName: "FT Yield Analysis",
		Version:     skills.Version{Major: 1, Minor: 0, Patch: 0},
		Description: "Calculates and verifies yield for Final Test data.",
		InputContract: []skills.FieldDefinition{
			{Name: "pass_column", Type: "string", Required: true, Description: "Column indicating PASS/FAIL"},
			{Name: "pass_value", Type: "string", Required: true, Description: "Value that means PASS"},
			{Name: "group_by", Type: "string", Required: false, Description: "Optional group-by column"},
		},
	}, func(ctx context.Context, input map[string]any) (any, error) {
		return "ft_yield_analysis executed", nil
	})
	skillReg.Register(skill)
	skillReg.ActivateLatest(skills.SkillFTYieldAnalysis)

	return &Orchestrator{
		Workflow:  workflow.NewTaskContext(cfg.TaskID, cfg.WorkDir),
		Skills:    skillReg,
		Hooks:     pipeline,
		Artifacts: artifacts.NewStore(),
		Executor:  cfg.Executor,
	}
}

// ── Pipeline Methods ─────────────────────────────────────────────────────

// Ingest reads the uploaded files and builds a Data Catalog.
func (o *Orchestrator) Ingest(ctx context.Context) error {
	if err := o.Workflow.Transition(ctx, workflow.StateIngesting); err != nil {
		return fmt.Errorf("transition to INGESTING: %w", err)
	}
	catalog, err := BuildCatalog(o.Workflow.WorkDir)
	if err != nil {
		return fmt.Errorf("build catalog: %w", err)
	}
	o.Catalog = catalog
	return o.Workflow.Transition(ctx, workflow.StateInspecting)
}

// Plan uses the executor to select a skill and generate a plan.
func (o *Orchestrator) Plan(ctx context.Context, task string) error {
	if err := o.Workflow.Transition(ctx, workflow.StatePlanning); err != nil {
		return fmt.Errorf("transition to PLANNING: %w", err)
	}
	// Generate plan via executor.
	planPrompt := fmt.Sprintf(
		"You are analyzing FT (Final Test) semiconductor data.\n\nTask: %s\n\nData Catalog:\n%s\n\nSelect the appropriate skill and generate a concise execution plan. The available skill is: ft_yield_analysis. Respond with a numbered plan for: 1) inspect workbook 2) calculate yield 3) verify yield 4) generate report.",
		task, o.Catalog.Summary(),
	)
	result, err := o.Executor.Execute(ctx, executor.Request{Task: planPrompt})
	if err != nil {
		return fmt.Errorf("generate plan: %w", err)
	}
	o.PlanText = result.Text

	// Record LLM usage.
	o.Workflow.RecordLLMCall(result.InputTokens, result.OutputTokens, 0)
	return nil
}

// WaitForApproval transitions to WAITING_APPROVAL and returns the plan.
// The caller must call Confirm() to proceed.
func (o *Orchestrator) WaitForApproval(ctx context.Context) error {
	return o.Workflow.Transition(ctx, workflow.StateWaitingApproval)
}

// Confirm transitions from WAITING_APPROVAL to EXECUTING.
func (o *Orchestrator) Confirm(ctx context.Context) error {
	if o.CurrentState() != workflow.StateWaitingApproval {
		return fmt.Errorf("cannot confirm: current state is %s, need WAITING_APPROVAL", o.CurrentState())
	}
	return o.Workflow.Transition(ctx, workflow.StateExecuting)
}

// ExecuteYield runs the deterministic yield calculation.
// Must be called after Confirm().
func (o *Orchestrator) ExecuteYield(ctx context.Context, filePath, sheetName string, params YieldParams) error {
	if o.CurrentState() != workflow.StateExecuting {
		return fmt.Errorf("cannot execute: current state is %s, need EXECUTING", o.CurrentState())
	}
	result, err := CalculateYield(filePath, sheetName, params)
	if err != nil {
		return fmt.Errorf("calculate yield: %w", err)
	}
	o.YieldResult = result
	o.YieldParams = params
	o.Workflow.RecordToolCall(int64(result.TotalCount))
	return nil
}

// Verify runs deterministic verification on the yield result.
func (o *Orchestrator) Verify(ctx context.Context, expectedCols []string) error {
	if err := o.Workflow.Transition(ctx, workflow.StateVerifying); err != nil {
		return fmt.Errorf("transition to VERIFYING: %w", err)
	}
	if o.YieldResult == nil {
		return fmt.Errorf("no yield result to verify")
	}
	o.Verification = VerifyYield(o.YieldResult, expectedCols)
	if !o.Verification.Passed {
		return fmt.Errorf("verification failed:\n%s", o.Verification.Summary())
	}
	return nil
}

// GenerateReport creates report.md and saves it as an artifact.
func (o *Orchestrator) GenerateReport(ctx context.Context, task string) error {
	if err := o.Workflow.Transition(ctx, workflow.StateReporting); err != nil {
		return fmt.Errorf("transition to REPORTING: %w", err)
	}
	if !o.Verification.Passed {
		return fmt.Errorf("cannot generate report: verification did not pass")
	}
	report := GenerateReport(ReportData{
		TaskID:       o.Workflow.TaskID,
		Task:         task,
		PlanSummary:  o.PlanText,
		Catalog:      o.Catalog,
		YieldResult:  o.YieldResult,
		Verification: o.Verification,
		ArtifactRefs: o.artifactRefs,
		GeneratedAt:  time.Now(),
	})
	reportPath := "artifacts/report.md"
	artID := artifacts.NewArtifactID()
	if err := o.Artifacts.Put(&artifacts.Artifact{
		ID:          artID,
		TaskID:      o.Workflow.TaskID,
		Type:        artifacts.ArtifactTypeReport,
		Name:        "FT Yield Report",
		Path:        reportPath,
		ContentType: "text/markdown",
		Size:        int64(len(report)),
		Metadata:    map[string]any{"report": report},
	}); err != nil {
		return fmt.Errorf("save report: %w", err)
	}
	o.artifactRefs = append(o.artifactRefs, artID)
	o.ReportPath = reportPath
	return nil
}

// Complete transitions to DONE and saves a final snapshot.
func (o *Orchestrator) Complete(ctx context.Context) error {
	if err := o.Workflow.Transition(ctx, workflow.StateDone); err != nil {
		return fmt.Errorf("transition to DONE: %w", err)
	}
	// Save final snapshot.
	_, err := o.Artifacts.CreateSnapshot(o.Workflow.TaskID, string(workflow.StateDone), "FT yield analysis complete")
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	return nil
}

// CurrentState returns the current workflow state.
func (o *Orchestrator) CurrentState() workflow.State {
	return o.Workflow.CurrentState()
}

// Fail transitions to FAILED.
func (o *Orchestrator) Fail(ctx context.Context, reason string) error {
	return o.Workflow.Transition(ctx, workflow.StateFailed)
}

// SaveArtifact is a helper to persist a result as an artifact.
func (o *Orchestrator) SaveArtifact(name, path, contentType string, data string) (string, error) {
	artID := artifacts.NewArtifactID()
	if err := o.Artifacts.Put(&artifacts.Artifact{
		ID:          artID,
		TaskID:      o.Workflow.TaskID,
		Type:        artifacts.ArtifactTypeResult,
		Name:        name,
		Path:        path,
		ContentType: contentType,
		Size:        int64(len(data)),
		Metadata:    map[string]any{"data": data},
	}); err != nil {
		return "", fmt.Errorf("save artifact %q: %w", name, err)
	}
	o.artifactRefs = append(o.artifactRefs, artID)
	return artID, nil
}

// IsWaitingApproval returns true if the workflow is at WAITING_APPROVAL.
func (o *Orchestrator) IsWaitingApproval() bool {
	return o.CurrentState() == workflow.StateWaitingApproval
}

// ExecutionPlan returns a human-readable plan string.
func (o *Orchestrator) ExecutionPlan() string {
	if o.PlanText == "" {
		return "(no plan)"
	}
	return strings.TrimSpace(o.PlanText)
}
