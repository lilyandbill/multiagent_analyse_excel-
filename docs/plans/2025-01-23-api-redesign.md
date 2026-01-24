# API Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 重构 Excel 处理 API，将 upload + process 合并为单一 /analyze 接口，支持同步和异步模式

**Architecture:** 在 Service 层添加新的 `AnalyzeExcel` 方法，根据 `async` 参数决定同步还是异步处理。Handler 层添加新的 `/analyze` 端点。保持任务管理功能不变。

**Tech Stack:** Go 1.25, Gin Web Framework, CloudWeGo Eino Agent Framework

---

## Task 1: Service Layer - Add AnalyzeExcel Method

**Files:**
- Modify: `service/excel_service.go`

**Step 1: Add AnalyzeExcel method signature and async support**

在 `ExcelService` 结构体后添加新方法：

```go
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
	result, err := s.ProcessExcel(ctx, taskID, "")
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

// AnalysisResult 分析结果
type AnalysisResult struct {
	TaskID      string                 `json:"task_id"`
	Status      TaskStatus             `json:"status"`
	Result      interface{}            `json:"result,omitempty"`
	DownloadURL string                 `json:"download_url,omitempty"`
	QueryURL    string                 `json:"query_url,omitempty"`
}
```

插入位置：在 `GetTaskCount` 方法之后，`service/excel_service.go:528`

**Step 2: Verify compilation**

Run: `go build -o /dev/null ./service`
Expected: No errors

**Step 3: Commit**

```bash
git add service/excel_service.go
git commit -m "feat: add AnalyzeExcel method to support sync/async analysis"
```

---

## Task 2: Service Layer - Support Async Processing in UploadExcel

**Files:**
- Modify: `service/excel_service.go`

**Step 1: Modify UploadExcel to return taskID only**

当前 `UploadExcel` 已经返回 taskID，无需修改。确认方法签名正确：

```go
func (s *ExcelService) UploadExcel(ctx context.Context, filename string, fileContent []byte, prompt string) (string, error)
```

**Step 2: Add background processing for async mode**

在 `AnalyzeExcel` 方法中添加异步处理逻辑：

```go
// 2. 异步模式：立即返回 task_id，后台处理
if async {
	go func() {
		asyncCtx := context.Background()
		_, err := s.ProcessExcel(asyncCtx, taskID, "")
		if err != nil {
			logger.Error("异步处理失败", zap.String("task_id", taskID), zap.Error(err))
		}
	}()

	return &AnalysisResult{
		TaskID:   taskID,
		Status:   TaskStatusProcessing,
		QueryURL: fmt.Sprintf("/api/v1/excel/task/%s", taskID),
	}, nil
}
```

**Step 3: Verify compilation**

Run: `go build -o /dev/null ./service`
Expected: No errors

**Step 4: Commit**

```bash
git add service/excel_service.go
git commit -m "feat: add background goroutine for async processing"
```

---

## Task 3: Handler Layer - Add AnalyzeExcel Handler

**Files:**
- Modify: `api/handler/excel_handler.go`

**Step 1: Add AnalyzeExcel request struct**

在 `ProcessExcelRequest` 后添加：

```go
// AnalyzeExcelRequest 分析 Excel 请求
type AnalyzeExcelRequest struct {
	// 文件和 prompt 从 form data 获取
	Async bool `form:"async"` // 是否异步处理
}
```

插入位置：`api/handler/excel_handler.go:49`

**Step 2: Add AnalyzeExcel handler method**

在 `DeleteTask` 方法之后添加新方法：

```go
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

	newResponse(c, true, 0, "分析成功", result)
}
```

插入位置：`api/handler/excel_handler.go:344`（在 DeleteTask 之后）

**Step 3: Verify compilation**

Run: `go build -o /dev/null ./api/handler`
Expected: No errors

**Step 4: Commit**

```bash
git add api/handler/excel_handler.go
git commit -m "feat: add AnalyzeExcel handler with sync/async support"
```

---

## Task 4: Router Layer - Add New Route

**Files:**
- Modify: `api/router/router.go`

**Step 1: Remove old routes**

删除以下路由：
- `excel.POST("/upload", r.excelHandler.UploadExcel)`
- `excel.POST("/process", r.excelHandler.ProcessExcel)`
- `excel.POST("/process/async", r.excelHandler.ProcessExcelAsync)`

**Step 2: Add new /analyze route**

在 `SetupRoutes` 方法的 `excel` 路由组中添加：

```go
excel := v1.Group("/excel")
{
	// 分析接口
	excel.POST("/analyze", r.excelHandler.AnalyzeExcel)

	// 任务操作
	excel.GET("/tasks", r.excelHandler.ListTasks)
	excel.GET("/task/:task_id", r.excelHandler.GetTaskStatus)
	excel.GET("/preview/:task_id", r.excelHandler.PreviewFile)
	excel.GET("/download/:task_id", r.excelHandler.DownloadResult)
	excel.DELETE("/task/:task_id", r.excelHandler.DeleteTask)
}
```

完整修改后的 `SetupRoutes` 方法：

```go
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

		// 统计接口
		v1.GET("/stats", r.getStats)
	}
}
```

**Step 3: Update stats endpoint**

更新 `getStats` 方法中的端点列表：

```go
func (r *Router) getStats(c *gin.Context) {
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
```

**Step 4: Verify compilation**

Run: `go build -o /dev/null .`
Expected: No errors

**Step 5: Commit**

```bash
git add api/router/router.go
git commit -m "refactor: replace old routes with new /analyze endpoint"
```

---

## Task 5: Update Test Script

**Files:**
- Modify: `test_api.sh`

**Step 1: Rewrite test script for new API**

```bash
#!/bin/bash

# 测试脚本 - 新 API
BASE_URL="http://localhost:8080/api/v1"

echo "===== 测试 Excel 分析接口 ====="
echo ""

# 1. 测试同步模式
echo "1. 测试同步分析..."
# 创建一个测试文件
echo '{"name":"test","value":"123"}' > /tmp/test.json

SYNC_RESP=$(curl -s -X POST "$BASE_URL/excel/analyze" \
  -F "file=@/tmp/test.json" \
  -F "prompt=分析这个文件" \
  -F "async=false")

echo "同步响应: $SYNC_RESP"
echo ""

# 提取 task_id
TASK_ID=$(echo $SYNC_RESP | grep -o '"task_id":"[^"]*' | cut -d'"' -f4)

if [ -z "$TASK_ID" ]; then
  echo "错误: 无法获取 task_id"
  echo "请先确保服务器正在运行: go run main.go"
  exit 1
fi

echo "获取到 task_id: $TASK_ID"
echo ""

# 2. 测试异步模式
echo "2. 测试异步分析..."
ASYNC_RESP=$(curl -s -X POST "$BASE_URL/excel/analyze" \
  -F "file=@/tmp/test.json" \
  -F "prompt=异步分析这个文件" \
  -F "async=true")

echo "异步响应: $ASYNC_RESP"
echo ""

# 等待异步任务完成
echo "3. 等待 2 秒后查询异步任务状态..."
sleep 2

STATUS_RESP=$(curl -s -X GET "$BASE_URL/excel/task/$TASK_ID")
echo "任务状态: $STATUS_RESP"
echo ""

# 4. 测试任务列表
echo "4. 查询任务列表..."
TASKS_RESP=$(curl -s -X GET "$BASE_URL/excel/tasks?page=1&page_size=10")
echo "任务列表: $TASKS_RESP"
echo ""

echo "===== 测试完成 ====="
```

**Step 2: Commit**

```bash
git add test_api.sh
git commit -m "test: update test script for new /analyze API"
```

---

## Task 6: Update CLAUDE.md Documentation

**Files:**
- Modify: `CLAUDE.md`

**Step 1: Update API Endpoints section**

替换 "## API Endpoints" 部分为：

```markdown
## API Endpoints

- `POST /api/v1/excel/analyze` - 上传文件并分析（支持同步/异步）
  - 参数：file（文件）、prompt（问题）、async（是否异步，默认 false）
  - 同步模式：直接返回分析结果和下载链接
  - 异步模式：返回 task_id 和查询 URL
- `GET /api/v1/excel/task/{task_id}` - 查询任务状态和结果
- `GET /api/v1/excel/download/{task_id}` - 下载结果文件
- `GET /api/v1/excel/tasks` - 列出所有任务（支持分页和状态筛选）
- `GET /api/v1/excel/preview/{task_id}` - 预览文件内容
- `DELETE /api/v1/excel/task/{task_id}` - 删除任务
```

**Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update API documentation for new /analyze endpoint"
```

---

## Task 7: Integration Testing

**Files:**
- None (testing)

**Step 1: Start the server**

Run: `go run main.go`
Expected: Server starts at http://localhost:8080

**Step 2: Run test script in another terminal**

Run: `bash test_api.sh`
Expected:
- Sync request returns completed task with results
- Async request returns task_id with processing status
- Task query returns task details
- Task list returns all tasks

**Step 3: Test error scenarios**

```bash
# Test missing file
curl -X POST "http://localhost:8080/api/v1/excel/analyze" \
  -F "prompt=test"

# Test missing prompt
curl -X POST "http://localhost:8080/api/v1/excel/analyze" \
  -F "file=@/tmp/test.json"

# Test invalid task_id
curl -X GET "http://localhost:8080/api/v1/excel/task/invalid-id"
```

Expected: 400/404 errors with clear messages

**Step 4: Stop server and commit**

```bash
git add -A
git commit -m "test: verify integration testing passes"
```

---

## Task 8: Update README.md

**Files:**
- Modify: `README.md`

**Step 1: Update API interface documentation**

替换 "## API 接口" 部分为：

```markdown
## API 接口

| 接口 | 方法 | 描述 |
|------|------|------|
| `/api/v1/excel/analyze` | POST | 上传并分析 Excel 文件（同步/异步） |
| `/api/v1/excel/task/{task_id}` | GET | 查询任务状态 |
| `/api/v1/excel/download/{task_id}` | GET | 下载结果文件 |
| `/api/v1/excel/tasks` | GET | 列出所有任务（分页） |
| `/api/v1/excel/preview/{task_id}` | GET | 预览文件内容 |
| `/api/v1/excel/task/{task_id}` | DELETE | 删除任务 |

### 使用示例

**同步分析（推荐用于简单任务）：**
\`\`\`bash
curl -X POST "http://localhost:8080/api/v1/excel/analyze" \
  -F "file=@data.xlsx" \
  -F "prompt=计算销售额总和" \
  -F "async=false"
\`\`\`

**异步分析（推荐用于复杂任务）：**
\`\`\`bash
curl -X POST "http://localhost:8080/api/v1/excel/analyze" \
  -F "file=@data.xlsx" \
  -F "prompt=生成数据透视表和图表" \
  -F "async=true"
\`\`\`
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README with new API usage examples"
```

---

## Task 9: Final Verification and Cleanup

**Files:**
- Multiple

**Step 1: Full build test**

Run:
```bash
go mod tidy
go build -o excel-agent main.go
./excel-agent &
SERVER_PID=$!
sleep 2
bash test_api.sh
kill $SERVER_PID
```

Expected: All tests pass

**Step 2: Check for unused code**

确认以下方法可以删除或标记为 deprecated：
- `UploadExcel` handler（不再需要，功能合并到 AnalyzeExcel）
- `ProcessExcel` handler（不再需要，功能合并到 AnalyzeExcel）
- `ProcessExcelAsync` handler（不再需要，功能合并到 AnalyzeExcel）

但是保留 Service 层的 `UploadExcel` 和 `ProcessExcel` 方法，因为新的 `AnalyzeExcel` 依赖它们。

**Step 3: Final commit**

```bash
git add -A
git commit -m "feat: complete API redesign implementation

- Add AnalyzeExcel service method with sync/async support
- Add /analyze endpoint replacing upload/process endpoints
- Update documentation and test scripts
- All integration tests passing"
```

---

## Task 10: Merge to Main

**Files:**
- None (git operations)

**Step 1: Switch back to main branch**

Run: `cd /Users/huangshengxue/code/golang/excel-agent && git checkout main`

**Step 2: Merge feature branch**

Run:
```bash
git merge feature/api-redesign --no-ff -m "Merge feature/api-redesign: Implement new /analyze API

Consolidates upload + process into single endpoint with sync/async support"
```

**Step 3: Push to GitHub**

Run: `git push origin main`

**Step 4: Cleanup worktree**

Run:
```bash
git worktree remove ../excel-agent-api-redesign
git branch -d feature/api-redesign
```

---

## Testing Checklist

- [ ] Sync mode returns completed task with results
- [ ] Async mode returns task_id with processing status
- [ ] Task query returns correct status and results
- [ ] Task list pagination works correctly
- [ ] File download returns correct file
- [ ] Task deletion removes all files
- [ ] Error handling works for missing parameters
- [ ] Error handling works for invalid task_id
- [ ] Concurrent requests handled correctly
- [ ] Server starts without errors
- [ ] All compilation succeeds
- [ ] Test script passes completely

## Notes

- 新 API 完全向后不兼容，但更简洁易用
- Service 层复用了现有的 `UploadExcel` 和 `ProcessExcel` 方法
- 异步处理使用 goroutine，无需额外的队列系统
- 任务保留 24 小时，每小时自动清理
