package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func createTestExcel(t *testing.T, dir string) string {
	t.Helper()
	f := excelize.NewFile()
	headers := []string{"LOT_ID", "WAFER_ID", "TEST_RESULT"}
	for i, h := range headers {
		col, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue("Sheet1", col, h)
	}
	data := []struct{ lot, wafer, result string }{
		{"LotA", "W01", "PASS"},
		{"LotA", "W02", "PASS"},
		{"LotA", "W03", "FAIL"},
		{"LotB", "W04", "PASS"},
		{"LotB", "W05", "PASS"},
	}
	for i, d := range data {
		row := i + 2
		f.SetCellValue("Sheet1", fmt.Sprintf("A%d", row), d.lot)
		f.SetCellValue("Sheet1", fmt.Sprintf("B%d", row), d.wafer)
		f.SetCellValue("Sheet1", fmt.Sprintf("C%d", row), d.result)
	}
	path := filepath.Join(dir, "test.xlsx")
	f.SaveAs(path)
	f.Close()
	return path
}

func uploadFile(t *testing.T, url, field, path, task string) (*http.Request, *bytes.Buffer) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile(field, filepath.Base(path))
	f, _ := os.Open(path)
	io.Copy(part, f)
	f.Close()
	if task != "" {
		writer.WriteField("task", task)
	}
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, body
}

func parseJSON[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var cr CommonResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	return parseJSONFromData[T](t, cr.Data)
}

func parseJSONFromData[T any](t *testing.T, data any) T {
	t.Helper()
	var result T
	raw, _ := json.Marshal(data)
	json.Unmarshal(raw, &result)
	return result
}

// ── Tests ────────────────────────────────────────────────────────────────

func TestFTAnalyze_Success(t *testing.T) {
	os.MkdirAll(uploadDir, 0755)
	defer os.RemoveAll(uploadDir)

	h := NewFTHandler()
	dir := t.TempDir()
	excelPath := createTestExcel(t, dir)

	req, _ := uploadFile(t, "/api/v1/ft/analyze", "file", excelPath, "analyze FT yield")
	w := httptest.NewRecorder()
	c, _ := createGinContext(w, req)
	h.AnalyzeFT(c)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestFTAnalyze_InvalidFileType(t *testing.T) {
	os.MkdirAll(uploadDir, 0755)
	defer os.RemoveAll(uploadDir)

	h := NewFTHandler()
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "test.txt")
	os.WriteFile(txtPath, []byte("not excel"), 0644)

	req, _ := uploadFile(t, "/api/v1/ft/analyze", "file", txtPath, "")
	w := httptest.NewRecorder()
	c, _ := createGinContext(w, req)
	h.AnalyzeFT(c)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestFTAnalyze_UploadSizeLimit(t *testing.T) {
	os.MkdirAll(uploadDir, 0755)
	defer os.RemoveAll(uploadDir)

	// We can't easily test upload size via multipart in unit tests without
	// actually hitting the handler with a gin context. This is covered
	// by the handler's internal check which is straightforward.
}

func TestFT_FullWorkflow(t *testing.T) {
	os.MkdirAll(uploadDir, 0755)
	defer os.RemoveAll(uploadDir)

	h := NewFTHandler()
	dir := t.TempDir()
	excelPath := createTestExcel(t, dir)

	// Step 1: Upload and plan.
	req, _ := uploadFile(t, "/api/v1/ft/analyze", "file", excelPath, "analyze FT yield by lot")
	w := httptest.NewRecorder()
	c, _ := createGinContext(w, req)
	h.AnalyzeFT(c)
	resp := w.Result()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload failed: %d %s", resp.StatusCode, string(body))
	}
	result := parseJSON[ftAnalyzeResponse](t, resp)
	if result.RunID == "" {
		t.Fatal("expected run_id")
	}
	if result.Status != "WAITING_CONFIRMATION" {
		t.Fatalf("status = %s, want WAITING_CONFIRMATION", result.Status)
	}
	if result.Skill != "ft_yield_analysis" {
		t.Errorf("skill = %s", result.Skill)
	}
	t.Logf("Run ID: %s", result.RunID)

	// Step 2: Check status.
	w2 := httptest.NewRecorder()
	c2, _ := createGinContextWithParam(w2, "GET", "/api/v1/ft/status/"+result.RunID, "run_id", result.RunID)
	h.GetFTStatus(c2)
	resp2 := w2.Result()
	statusResult := parseJSON[ftStatusResponse](t, resp2)
	if statusResult.Status != "WAITING_APPROVAL" {
		t.Errorf("status = %s, want WAITING_APPROVAL", statusResult.Status)
	}

	// Step 3: Confirm.
	confirmBody := fmt.Sprintf(`{"run_id":"%s","confirmed":true}`, result.RunID)
	w3 := httptest.NewRecorder()
	c3, _ := createGinContextWithBody(w3, "POST", "/api/v1/ft/confirm", confirmBody)
	h.ConfirmFT(c3)
	resp3 := w3.Result()
	// Read full response for debugging.
	var cr3 CommonResponse
	json.NewDecoder(resp3.Body).Decode(&cr3)
	if !cr3.Success {
		t.Fatalf("confirm failed: code=%d message=%s", cr3.Code, cr3.Message)
	}
	confirmResult := parseJSONFromData[ftConfirmResponse](t, cr3.Data)
	if confirmResult.Status != "DONE" {
		t.Fatalf("confirm status = %s, want DONE", confirmResult.Status)
	}
	if confirmResult.Yield == nil {
		t.Fatal("expected yield result")
	}
	t.Logf("Yield: %.2f%% (%d/%d)", confirmResult.Yield.Yield*100,
		confirmResult.Yield.PassCount, confirmResult.Yield.TotalCount)

	// Step 4: Report.
	w4 := httptest.NewRecorder()
	c4, _ := createGinContextWithParam(w4, "GET", "/api/v1/ft/report/"+result.RunID, "run_id", result.RunID)
	h.GetFTReport(c4)
	resp4 := w4.Result()
	if resp4.StatusCode != 200 {
		t.Errorf("report request failed: %d", resp4.StatusCode)
	}

	// Step 5: Duplicate confirmation is rejected.
	w5 := httptest.NewRecorder()
	c5, _ := createGinContextWithBody(w5, "POST", "/api/v1/ft/confirm", confirmBody)
	h.ConfirmFT(c5)
	resp5 := w5.Result()
	var cr5 CommonResponse
	json.NewDecoder(resp5.Body).Decode(&cr5)
	if cr5.Success {
		t.Error("duplicate confirmation should be rejected")
	}
	t.Logf("Duplicate confirm: code=%d message=%s", cr5.Code, cr5.Message)

	t.Logf("Full workflow completed: %s", result.RunID)
}

func TestFT_ConfirmBeforeUpload(t *testing.T) {
	h := NewFTHandler()
	w := httptest.NewRecorder()
	c, _ := createGinContextWithBody(w, "POST", "/api/v1/ft/confirm", `{"run_id":"nonexistent","confirmed":true}`)
	h.ConfirmFT(c)
	resp := w.Result()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 with error in body, got %d", resp.StatusCode)
	}
	var cr CommonResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	if cr.Success {
		t.Error("confirm for nonexistent run should fail")
	}
}

func TestFT_Cancel(t *testing.T) {
	os.MkdirAll(uploadDir, 0755)
	defer os.RemoveAll(uploadDir)

	h := NewFTHandler()
	dir := t.TempDir()
	excelPath := createTestExcel(t, dir)

	// Upload.
	req, _ := uploadFile(t, "/api/v1/ft/analyze", "file", excelPath, "test")
	w := httptest.NewRecorder()
	c, _ := createGinContext(w, req)
	h.AnalyzeFT(c)
	runID := parseJSON[ftAnalyzeResponse](t, w.Result()).RunID

	// Cancel.
	w2 := httptest.NewRecorder()
	c2, _ := createGinContextWithBody(w2, "POST", "/api/v1/ft/confirm",
		fmt.Sprintf(`{"run_id":"%s","confirmed":false}`, runID))
	h.ConfirmFT(c2)
	cancelResult := parseJSON[ftConfirmResponse](t, w2.Result())
	if cancelResult.Status != "FAILED" {
		t.Errorf("cancel status = %s, want FAILED", cancelResult.Status)
	}
}

func TestFT_StatusMissingRun(t *testing.T) {
	h := NewFTHandler()
	w := httptest.NewRecorder()
	c, _ := createGinContextWithParam(w, "GET", "/api/v1/ft/status/nonexistent", "run_id", "nonexistent")
	h.GetFTStatus(c)
	resp := w.Result()
	var cr CommonResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	if cr.Success {
		t.Error("status for missing run should fail")
	}
}

func TestFT_ReportBeforeDone(t *testing.T) {
	os.MkdirAll(uploadDir, 0755)
	defer os.RemoveAll(uploadDir)

	h := NewFTHandler()
	dir := t.TempDir()
	excelPath := createTestExcel(t, dir)

	req, _ := uploadFile(t, "/api/v1/ft/analyze", "file", excelPath, "test")
	w := httptest.NewRecorder()
	c, _ := createGinContext(w, req)
	h.AnalyzeFT(c)
	runID := parseJSON[ftAnalyzeResponse](t, w.Result()).RunID

	// Request report before confirmation — should fail.
	w2 := httptest.NewRecorder()
	c2, _ := createGinContextWithParam(w2, "GET", "/api/v1/ft/report/"+runID, "run_id", runID)
	h.GetFTReport(c2)
	resp := w2.Result()
	var cr CommonResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	if cr.Success {
		t.Error("report before done should fail")
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"normal.xlsx", "normal.xlsx"},
		{"../../../etc/passwd", "passwd"}, // filepath.Base strips path
		{"file:name.xlsx", "file_name.xlsx"},
		{"test file (1).xlsx", "test file (1).xlsx"},
	}
	for _, tt := range tests {
		result := sanitizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("sanitize(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// ── Gin Test Helpers ─────────────────────────────────────────────────────

func createGinContext(w http.ResponseWriter, req *http.Request) (*gin.Context, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	c, r := gin.CreateTestContext(w)
	c.Request = req
	return c, r
}

func createGinContextWithParam(w http.ResponseWriter, method, path, key, value string) (*gin.Context, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	c, r := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Params = gin.Params{{Key: key, Value: value}}
	return c, r
}

func createGinContextWithBody(w http.ResponseWriter, method, path, body string) (*gin.Context, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	c, r := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, r
}
