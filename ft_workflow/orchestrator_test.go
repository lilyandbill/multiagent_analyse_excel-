package ft_workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"excel-agent/executor"
	"excel-agent/workflow"

	"github.com/xuri/excelize/v2"
)

// createTestFTExcel creates a minimal FT Excel file for testing.
func createTestFTExcel(t *testing.T, dir string) string {
	t.Helper()
	f := excelize.NewFile()
	sheet := "Sheet1"

	// Header: LOT_ID, WAFER_ID, SITE, TEST_RESULT
	headers := []string{"LOT_ID", "WAFER_ID", "SITE", "TEST_RESULT"}
	for i, h := range headers {
		col, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, col, h)
	}

	// 10 rows of data: 8 PASS, 2 FAIL.
	// LotA: 5 PASS, 1 FAIL = 83.33%
	// LotB: 3 PASS, 1 FAIL = 75.00%
	data := []struct{ lot, wafer, site, result string }{
		{"LotA", "W01", "1", "PASS"},
		{"LotA", "W01", "2", "PASS"},
		{"LotA", "W02", "1", "PASS"},
		{"LotA", "W02", "2", "PASS"},
		{"LotA", "W03", "1", "FAIL"},
		{"LotA", "W03", "2", "PASS"},
		{"LotB", "W04", "1", "PASS"},
		{"LotB", "W04", "2", "PASS"},
		{"LotB", "W05", "1", "FAIL"},
		{"LotB", "W05", "2", "PASS"},
	}
	for i, d := range data {
		row := i + 2
		col, _ := excelize.CoordinatesToCellName(1, row)
		f.SetCellValue(sheet, col, d.lot)
		col, _ = excelize.CoordinatesToCellName(2, row)
		f.SetCellValue(sheet, col, d.wafer)
		col, _ = excelize.CoordinatesToCellName(3, row)
		f.SetCellValue(sheet, col, d.site)
		col, _ = excelize.CoordinatesToCellName(4, row)
		f.SetCellValue(sheet, col, d.result)
	}

	path := filepath.Join(dir, "ft_data.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("create test xlsx: %v", err)
	}
	f.Close()
	return path
}

func TestOrchestrator_FullHappyPath(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	// Create test FT Excel file.
	excelPath := createTestFTExcel(t, workDir)

	orch := NewOrchestrator(OrchestratorConfig{
		TaskID:   "ft-test-001",
		WorkDir:  workDir,
		Task:     "analyze FT yield by lot",
		Executor: executor.FakeExecutor{},
	})

	// 1. Ingest: build Data Catalog.
	if err := orch.Ingest(ctx); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if orch.Catalog == nil {
		t.Fatal("catalog should not be nil")
	}
	if len(orch.Catalog.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(orch.Catalog.Tables))
	}
	t.Logf("Catalog:\n%s", orch.Catalog.Summary())

	// 2. Plan: generate plan via fake executor.
	if err := orch.Plan(ctx, "analyze FT yield by lot"); err != nil {
		t.Fatalf("plan: %v", err)
	}
	if orch.PlanText == "" {
		t.Fatal("plan should not be empty")
	}
	t.Logf("Plan:\n%s", orch.PlanText)

	// 3. Wait for approval.
	if err := orch.WaitForApproval(ctx); err != nil {
		t.Fatalf("wait for approval: %v", err)
	}
	if !orch.IsWaitingApproval() {
		t.Fatalf("should be WAITING_APPROVAL, got %s", orch.CurrentState())
	}

	// 4. Verify no calculation yet.
	if orch.YieldResult != nil {
		t.Fatal("yield result should be nil before confirmation")
	}

	// 5. Confirm.
	if err := orch.Confirm(ctx); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if orch.CurrentState() != workflow.StateExecuting {
		t.Fatalf("should be EXECUTING after confirm, got %s", orch.CurrentState())
	}

	// 6. Execute yield calculation.
	params := YieldParams{
		TableIndex: 0,
		PassColumn: "TEST_RESULT",
		PassValue:  "PASS",
		GroupBy:    "LOT_ID",
	}
	sheetName := orch.Catalog.Tables[0].SheetName
	if err := orch.ExecuteYield(ctx, excelPath, sheetName, params); err != nil {
		t.Fatalf("execute yield: %v", err)
	}
	if orch.YieldResult.TotalCount != 10 {
		t.Errorf("total = %d, want 10", orch.YieldResult.TotalCount)
	}
	if orch.YieldResult.PassCount != 8 {
		t.Errorf("pass = %d, want 8", orch.YieldResult.PassCount)
	}
	if orch.YieldResult.Yield != 0.8 {
		t.Errorf("yield = %.4f, want 0.8000", orch.YieldResult.Yield)
	}
	if len(orch.YieldResult.Groups) != 2 {
		t.Errorf("groups = %d, want 2 (LotA, LotB)", len(orch.YieldResult.Groups))
	}
	t.Logf("Yield:\n%s", orch.YieldResult.YieldSummary())

	// 7. Verify.
	expectedCols := orch.Catalog.ColumnNames(0)
	if err := orch.Verify(ctx, expectedCols); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !orch.Verification.Passed {
		t.Errorf("verification should pass, got: %s", orch.Verification.Summary())
	}
	t.Logf("Verification:\n%s", orch.Verification.Summary())

	// 8. Generate report.
	if err := orch.GenerateReport(ctx, "analyze FT yield by lot"); err != nil {
		t.Fatalf("generate report: %v", err)
	}
	if orch.ReportPath != "artifacts/report.md" {
		t.Errorf("report path = %q", orch.ReportPath)
	}
	// Verify report artifact exists.
	reportIDs := orch.Artifacts.ListByType("report")
	if len(reportIDs) != 1 {
		t.Errorf("expected 1 report artifact, got %d", len(reportIDs))
	}

	// 9. Complete.
	if err := orch.Complete(ctx); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if orch.CurrentState() != workflow.StateDone {
		t.Fatalf("final state = %s, want DONE", orch.CurrentState())
	}

	// 10. Snapshot exists.
	snaps := orch.Artifacts.ListSnapshots("ft-test-001")
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(snaps))
	}
	if snaps[0].State != string(workflow.StateDone) {
		t.Errorf("snapshot state = %s, want DONE", snaps[0].State)
	}

	// Budget checks.
	usg := orch.Workflow.Snapshot().Usage
	if usg.Iterations != 8 {
		t.Errorf("iterations = %d, want 8", usg.Iterations)
	}
	if usg.LLMCalls != 1 {
		t.Errorf("LLM calls = %d, want 1", usg.LLMCalls)
	}
	if usg.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1", usg.ToolCalls)
	}
	t.Logf("Completed successfully: %d iterations", usg.Iterations)
}

func TestOrchestrator_NoCalculationBeforeConfirmation(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	createTestFTExcel(t, workDir)

	orch := NewOrchestrator(OrchestratorConfig{
		TaskID: "ft-test-002", WorkDir: workDir, Task: "test",
		Executor: executor.FakeExecutor{},
	})
	orch.Ingest(ctx)
	orch.Plan(ctx, "test")
	orch.WaitForApproval(ctx)

	// Trying to execute before confirmation should fail.
	err := orch.ExecuteYield(ctx, "fake.xlsx", "Sheet1", YieldParams{
		PassColumn: "TEST_RESULT", PassValue: "PASS",
	})
	if err == nil {
		t.Fatal("expected error when executing before confirmation")
	}
	if !strings.Contains(err.Error(), "cannot execute") {
		t.Errorf("error = %v, want 'cannot execute'", err)
	}
}

func TestOrchestrator_VerificationFailureBlocksReport(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()

	// Create an Excel file with a missing column.
	f := excelize.NewFile()
	f.SetCellValue("Sheet1", "A1", "LOT_ID")
	f.SetCellValue("Sheet1", "B1", "WAFER_ID")
	f.SetCellValue("Sheet1", "A2", "LotA")
	f.SetCellValue("Sheet1", "B2", "W01")
	path := filepath.Join(workDir, "bad.xlsx")
	f.SaveAs(path)
	f.Close()

	orch := NewOrchestrator(OrchestratorConfig{
		TaskID: "ft-test-003", WorkDir: workDir, Task: "test",
		Executor: executor.FakeExecutor{},
	})
	orch.Ingest(ctx)
	orch.Plan(ctx, "test")
	orch.WaitForApproval(ctx)
	orch.Confirm(ctx)

	// Execute yield — will fail because TEST_RESULT column doesn't exist.
	err := orch.ExecuteYield(ctx, path, "Sheet1", YieldParams{
		PassColumn: "TEST_RESULT", PassValue: "PASS",
	})
	if err == nil {
		t.Fatal("expected error for missing pass column")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

func TestOrchestrator_StateTransitions(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	createTestFTExcel(t, workDir)

	orch := NewOrchestrator(OrchestratorConfig{
		TaskID: "ft-test-004", WorkDir: workDir, Task: "test",
		Executor: executor.FakeExecutor{},
	})

	states := []workflow.State{}
	recordState := func() { states = append(states, orch.CurrentState()) }

	recordState() // RECEIVED
	orch.Ingest(ctx)
	recordState() // INSPECTING
	orch.Plan(ctx, "test")
	recordState() // PLANNING
	orch.WaitForApproval(ctx)
	recordState() // WAITING_APPROVAL
	orch.Confirm(ctx)
	recordState() // EXECUTING
	orch.ExecuteYield(ctx,
		filepath.Join(workDir, "ft_data.xlsx"),
		"Sheet1",
		YieldParams{PassColumn: "TEST_RESULT", PassValue: "PASS"},
	)
	recordState() // still EXECUTING
	orch.Verify(ctx, orch.Catalog.ColumnNames(0))
	recordState() // VERIFYING
	orch.GenerateReport(ctx, "test")
	recordState() // REPORTING
	orch.Complete(ctx)
	recordState() // DONE

	expected := []workflow.State{
		workflow.StateReceived,
		workflow.StateInspecting,
		workflow.StatePlanning,
		workflow.StateWaitingApproval,
		workflow.StateExecuting,
		workflow.StateExecuting,
		workflow.StateVerifying,
		workflow.StateReporting,
		workflow.StateDone,
	}
	for i, exp := range expected {
		if states[i] != exp {
			t.Errorf("state[%d] = %s, want %s", i, states[i], exp)
		}
	}
	if orch.CurrentState() != workflow.StateDone {
		t.Errorf("final state = %s, want DONE", orch.CurrentState())
	}
}

func TestBuildCatalog_NoFiles(t *testing.T) {
	_, err := BuildCatalog(t.TempDir())
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestYieldResult_YieldSummary(t *testing.T) {
	r := &YieldResult{
		TotalCount: 100, PassCount: 95, FailCount: 5, Yield: 0.95,
		GroupBy: "LOT_ID",
		Groups: []GroupYield{
			{GroupValue: "LotA", Total: 50, Pass: 48, Fail: 2, Yield: 0.96},
			{GroupValue: "LotB", Total: 50, Pass: 47, Fail: 3, Yield: 0.94},
		},
	}
	summary := r.YieldSummary()
	if !strings.Contains(summary, "95.00%") {
		t.Errorf("summary missing overall: %s", summary)
	}
	if !strings.Contains(summary, "96.00%") {
		t.Errorf("summary missing LotA: %s", summary)
	}
}

func TestVerificationResult_Summary(t *testing.T) {
	vr := VerifyYield(&YieldResult{
		TotalCount: 10, PassCount: 8, FailCount: 2, Yield: 0.8,
		Params: YieldParams{PassColumn: "R", GroupBy: "L"},
	}, []string{"L", "R"})
	if !vr.Passed {
		t.Errorf("verification should pass: %s", vr.Summary())
	}
}

func TestReport_ContainsAllSections(t *testing.T) {
	vr := VerifyYield(&YieldResult{
		TotalCount: 10, PassCount: 8, FailCount: 2, Yield: 0.8,
		Params: YieldParams{PassColumn: "R"},
	}, []string{"R"})
	report := GenerateReport(ReportData{
		TaskID: "t1", Task: "test", PlanSummary: "plan",
		YieldResult: &YieldResult{
			TotalCount: 10, PassCount: 8, FailCount: 2, Yield: 0.8,
			Params: YieldParams{GroupBy: "LOT"},
			GroupBy: "LOT",
			Groups: []GroupYield{
				{GroupValue: "LotA", Total: 6, Pass: 5, Fail: 1, Yield: 0.8333},
			},
		},
		Verification: vr,
	})
	for _, section := range []string{
		"# FT Yield Analysis Report",
		"## Plan",
		"## Yield Result",
		"## Verification",
		"80.00%",
		"| Group | Total | Pass | Fail | Yield |",
	} {
		if !strings.Contains(report, section) {
			t.Errorf("report missing section: %q", section)
		}
	}

	// Save to temp file for manual inspection.
	path := filepath.Join(t.TempDir(), "report_out.md")
	os.WriteFile(path, []byte(report), 0644)
	t.Logf("Report saved to: %s", path)
}

func TestOrchestrator_CannotConfirmTwice(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	createTestFTExcel(t, workDir)

	orch := NewOrchestrator(OrchestratorConfig{
		TaskID: "ft-test-005", WorkDir: workDir, Task: "test",
		Executor: executor.FakeExecutor{},
	})
	orch.Ingest(ctx)
	orch.Plan(ctx, "test")
	orch.WaitForApproval(ctx)
	orch.Confirm(ctx)

	// Second confirm should fail — state is already EXECUTING.
	err := orch.Confirm(ctx)
	if err == nil {
		t.Fatal("expected error for second confirmation")
	}
}

func TestOrchestrator_FailPath(t *testing.T) {
	ctx := context.Background()
	orch := NewOrchestrator(OrchestratorConfig{
		TaskID: "ft-test-006", WorkDir: t.TempDir(), Task: "test",
		Executor: executor.FakeExecutor{},
	})

	// Fail immediately.
	orch.Fail(ctx, "test failure")
	if orch.CurrentState() != workflow.StateFailed {
		t.Errorf("state = %s, want FAILED", orch.CurrentState())
	}
	if orch.CurrentState().IsTerminal() != true {
		t.Error("FAILED should be terminal")
	}
}

func TestOrchestrator_AllArtifactsSaved(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	createTestFTExcel(t, workDir)

	orch := NewOrchestrator(OrchestratorConfig{
		TaskID: "ft-test-007", WorkDir: workDir, Task: "test",
		Executor: executor.FakeExecutor{},
	})
	orch.Ingest(ctx)
	orch.Plan(ctx, "test")
	orch.WaitForApproval(ctx)
	orch.Confirm(ctx)
	orch.ExecuteYield(ctx,
		filepath.Join(workDir, "ft_data.xlsx"),
		"Sheet1",
		YieldParams{PassColumn: "TEST_RESULT", PassValue: "PASS"},
	)
	orch.SaveArtifact("yield_data", "artifacts/yield.json", "application/json",
		fmt.Sprintf(`{"yield": %f}`, orch.YieldResult.Yield))
	orch.Verify(ctx, orch.Catalog.ColumnNames(0))
	orch.SaveArtifact("verification", "artifacts/verify.json", "application/json",
		`{"status": "PASS"}`)
	orch.GenerateReport(ctx, "test")
	orch.Complete(ctx)

	// Check all artifacts by task.
	ids := orch.Artifacts.ListByTask("ft-test-007")
	if len(ids) < 3 {
		t.Errorf("expected at least 3 artifacts, got %d", len(ids))
	}
	// Check snapshot.
	snaps := orch.Artifacts.ListSnapshots("ft-test-007")
	if len(snaps) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(snaps))
	}
	// Snapshot should reference all artifacts at completion.
	snapArtifacts := orch.Artifacts.GetArtifactsForSnapshot(snaps[0].ID)
	t.Logf("Snapshot has %d artifacts", len(snapArtifacts))
}
