# Excel Analysis API Redesign

**Date:** 2025-01-23
**Author:** Claude Code
**Status:** Approved

## Overview

重构 Excel 处理接口，将现有的"上传 → 创建任务 → 启动任务"三步流程简化为"上传 + 提交问题 → 直接分析"的一站式接口。支持同步和异步两种处理模式。

## Design Goals

1. **简化用户体验** - 一个接口完成文件上传和分析任务提交
2. **灵活的处理模式** - 用户可选择同步（快速返回）或异步（后台处理）
3. **统一的数据格式** - 同步和异步返回相同的结构
4. **保留任务管理** - 支持历史任务查询、结果下载和删除

## API Design

### 1. Main Analysis Endpoint

**`POST /api/v1/excel/analyze`**

上传 Excel 文件并直接启动 AI 分析，支持同步和异步模式。

**Request:**
```
Content-Type: multipart/form-data

Parameters:
- file: Excel 文件（必填）
- prompt: 用户问题（必填）
- async: 是否异步处理（可选，默认 false）
  - false: 同步模式，等待处理完成后返回结果
  - true: 异步模式，立即返回 task_id
```

**Response - Sync Mode (async=false):**
```json
{
  "success": true,
  "data": {
    "task_id": "uuid",
    "status": "completed",
    "result": {
      "message": "AI 分析结果文本",
      "previews": [
        {
          "file_path": "path/to/file.xlsx",
          "single_file_previews": [...]
        }
      ],
      "files": [
        {
          "file_type": "file",
          "path": "result.xlsx",
          "type": "create"
        }
      ]
    },
    "download_url": "/api/v1/excel/download/{task_id}"
  }
}
```

**Response - Async Mode (async=true):**
```json
{
  "success": true,
  "data": {
    "task_id": "uuid",
    "status": "processing",
    "query_url": "/api/v1/excel/task/{task_id}",
    "message": "任务已提交，请通过 query_url 查询进度"
  }
}
```

**Error Responses:**
- `400 Bad Request` - 文件格式错误、参数缺失
- `500 Internal Server Error` - AI 处理失败
- `504 Gateway Timeout` - 同步处理超时

### 2. Task Query Endpoint

**`GET /api/v1/excel/task/:task_id`**

查询任务状态和获取处理结果。

**Response:**
```json
{
  "success": true,
  "data": {
    "task_id": "uuid",
    "filename": "original.xlsx",
    "prompt": "用户的问题",
    "status": "completed",  // pending | processing | completed | failed
    "created_at": "2025-01-23T10:00:00Z",
    "updated_at": "2025-01-23T10:05:00Z",
    "result": {
      "message": "分析结果",
      "previews": [...],
      "files": [...]
    },
    "download_url": "/api/v1/excel/download/{task_id}",
    "error": "错误信息（仅在 failed 状态）"
  }
}
```

**Status Values:**
- `pending` - 任务已创建，等待处理
- `processing` - 正在处理中
- `completed` - 处理完成
- `failed` - 处理失败（包含 error 字段）

### 3. Download Endpoint

**`GET /api/v1/excel/download/:task_id`**

下载处理结果文件（如果存在）。

**Response:**
- Success: 文件流（Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet）
- Not Found: 404 错误

### 4. Task List Endpoint

**`GET /api/v1/excel/tasks`**

获取任务列表，支持分页和状态筛选。

**Query Parameters:**
- `page`: 页码（默认 1）
- `page_size`: 每页数量（默认 10，最大 100）
- `status`: 状态筛选（可选，值：pending/processing/completed/failed）

**Response:**
```json
{
  "success": true,
  "data": {
    "tasks": [
      {
        "task_id": "uuid",
        "filename": "file.xlsx",
        "prompt": "问题",
        "status": "completed",
        "created_at": "2025-01-23T10:00:00Z",
        "updated_at": "2025-01-23T10:05:00Z"
      }
    ],
    "total": 100,
    "page": 1,
    "page_size": 10
  }
}
```

### 5. Task Deletion Endpoint

**`DELETE /api/v1/excel/task/:task_id`**

删除任务及其所有关联文件（上传文件、工作目录、结果文件）。

**Response:**
```json
{
  "success": true,
  "message": "任务删除成功"
}
```

## Implementation Strategy

### Service Layer Changes

1. **New Method: `AnalyzeExcel`**
   ```go
   func (s *ExcelService) AnalyzeExcel(
       ctx context.Context,
       filename string,
       fileContent []byte,
       prompt string,
       async bool,
   ) (*AnalysisResult, error)
   ```

2. **Sync Flow:**
   - Upload file → Create task → Process immediately → Return result
   - Use context with 30-minute timeout

3. **Async Flow:**
   - Upload file → Create task → Return task_id
   - Start background goroutine for processing
   - No timeout limit on background task

### Data Flow

```
Sync Request:
   User → API → Service:Upload → Service:Process → Agent → Result → Response

Async Request:
   User → API → Service:Upload → Response:task_id
                └─→ Background: goroutine → Service:Process → Agent → Update Task
```

### Error Handling

| Scenario | HTTP Code | Response Action |
|----------|-----------|-----------------|
| Invalid file format | 400 | 立即返回错误 |
| Missing parameters | 400 | 立即返回错误 |
| Upload failed | 500 | 立即返回错误 |
| AI processing failed | 500 | Mark task as failed, save error message |
| Sync timeout | 504 | Return task_id, continue processing in background |
| Partial success | 200 | Return intermediate results |

### Configuration

**Timeout Settings:**
- Sync mode: 30 minutes (configurable via `config.yaml`)
- Async mode: No timeout on processing

**Task Retention:**
- Tasks and files retained for 24 hours
- Automatic cleanup every hour
- Manual deletion via API

## Migration Plan

### Phase 1: Implementation
1. Create new `AnalyzeExcel` method in Service layer
2. Add new `/analyze` endpoint in API layer
3. Update router configuration

### Phase 2: Testing
1. Test sync mode with small files
2. Test async mode with large files
3. Test error scenarios
4. Load testing

### Phase 3: Cleanup (Optional)
1. Remove deprecated endpoints:
   - `POST /api/v1/excel/upload`
   - `POST /api/v1/excel/process`
   - `POST /api/v1/excel/process/async`
2. Update documentation

## Open Questions

- [ ] Should we add rate limiting per user?
- [ ] Should we support batch file upload?
- [ ] Should we add webhook support for async completion?

## References

- Current implementation: `service/excel_service.go`, `api/handler/excel_handler.go`
- Router configuration: `api/router/router.go`
- Agent system: `agents/` directory
