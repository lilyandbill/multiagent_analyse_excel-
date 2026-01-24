# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

```bash
# Install dependencies
go mod tidy

# Run the server (starts at http://localhost:8080)
go run main.go

# Build binary
go build -o excel-agent main.go

# Test API endpoints (requires server running)
bash test_api.sh
```

## Architecture

Excel Agent 是一个基于 LLM 的 Excel 智能处理系统，采用 **Plan-Execute 模式** 的多代理架构，基于 CloudWeGo Eino 框架构建。

### Core Layers

- **API Layer** (`api/`): Gin HTTP 服务，处理文件上传、任务管理和聊天接口
- **Service Layer** (`service/`): ExcelService 负责任务生命周期管理（创建/查询/删除）、文件存储和异步任务处理
- **Agent Layer** (`agents/`): 多代理协作系统
  - `planner/`: 将用户需求分解为执行计划（使用 JSON Schema 强制结构化输出）
  - `executor/`: 协调执行计划，可调用以下子代理：
    - `CodeAgent`: React 模式代理，生成并执行 Python 代码（pandas/matplotlib/openpyxl）
    - `WebSearchAgent`: 网络搜索辅助工具
  - `replanner/`: 根据执行结果动态调整计划
  - `report/`: 生成最终分析报告
- **Tools** (`tools/`): 供 Agent 调用的工具函数，包含预处理和后处理包装器：
  - Bash 命令执行（带文件变更检测）
  - Python 代码运行器（自动提取 markdown 代码块）
  - 文件读写、目录树查看
  - 所有工具支持 JSON 修复和输出格式化

### Data Flow

```
HTTP Request → Handler → ExcelService → Agent System
                                      ↓
                          Planner → Executor → [CodeAgent|WebSearchAgent|...]
                                      ↓
                              Replanner (if needed) → Report
                                      ↓
                              Result → Service → Response
```

### Key Types

- `ExcelTask` (`service/excel_service.go:49-60`): 任务实体，包含状态（pending/processing/completed/failed）、工作目录和结果文件路径
- `Plan` (`generic/plan.go:31-33`): 执行计划结构，包含有序步骤列表，每个步骤有 index 和 desc
- `Tool` wrappers (`tools/wrap.go:37-43`): 对基础工具进行预处理（JSON 修复）和后处理（格式化输出）

### Context Parameter System

项目使用 `params` 包管理上下文参数（基于 `sync.Map`）：
- `params.InitContextParams(ctx)`: 初始化上下文
- `params.AppendContextParams(ctx, values)`: 添加参数
- `params.GetTypedContextParams[T](ctx, key)`: 获取类型安全参数

关键 Session Keys（`params/consts.go:19-24`）：
- `FilePathSessionKey`: 文件路径
- `WorkDirSessionKey`: 工作目录
- `UserAllPreviewFilesSessionKey`: 文件预览信息
- `TaskIDKey`: 任务 ID

## Configuration

配置从 `config.yaml` 加载，支持环境变量覆盖：

```bash
# OpenAI 配置
OPENAI_API_KEY=sk-xxx
OPENAI_BASE_URL=https://...
OPENAI_MODEL=gpt-4

# ARK 配置（火山引擎）
ARK_API_KEY=sk-xxx
ARK_BASE_URL=https://...
ARK_MODEL=gpt-4
ARK_REGION=cn-beijing

# Python 解释器路径
EXCEL_AGENT_PYTHON_EXECUTABLE_PATH=/usr/bin/python3
```

## Agent System Details

### Plan-Execute 架构

1. **Planner Agent** (`agents/planner/planner.go:85-103`)
   - 使用 `planexecute.NewPlanner` 创建
   - 强制 JSON Schema 输出（`generic.PlanToolInfo`）
   - 将用户需求分解为结构化步骤

2. **Executor Agent** (`agents/executor/executor.go:50-100`)
   - 使用 `planexecute.NewExecutor` 创建
   - 可调用多个子代理工具（CodeAgent、WebSearchAgent）
   - 支持最多 20 次迭代

3. **Replanner Agent**
   - 根据执行结果动态调整计划
   - 处理失败或需要额外步骤的情况

4. **Sequential Agent** (`service/excel_service.go:562-573`)
   - 将 Plan-Execute Agent 和 Report Agent 串联
   - 确保完整的工作流程

### Tool Wrappers

工具包装器（`tools/wrap.go`）提供：
- **预处理**: `ToolRequestRepairJSON` - 自动修复 LLM 输出的 JSON 格式
- **后处理**:
  - `FilePostProcess`: 格式化命令执行结果，提取文件变更信息
  - `EditFilePostProcess`: 简化文件编辑响应

### CodeAgent 工作流

CodeAgent (`agents/executor/code_agent.go:34-111`) 使用 React 模式：
1. 接收清晰的 Excel 处理任务
2. 通过工具生成 Python 代码
3. 执行代码并返回结果
4. 支持最多 1000 次迭代

可用 Python 库：
- `pandas`: 数据分析和操作
- `matplotlib`: 绘图和可视化
- `openpyxl`: Excel 文件读写

## File Organization

```
excel-agent/
├── agents/          # 代理实现
│   ├── executor/    # 执行器和子代理
│   ├── planner/     # 规划器
│   ├── replanner/   # 重规划器
│   └── report/      # 报告生成
├── api/             # HTTP 服务层
├── config/          # 配置加载
├── excel/           # Excel 文件存储
│   ├── uploads/     # 上传文件（按 taskID 组织）
│   └── results/     # 结果文件
├── generic/         # 通用数据结构和工具信息
├── logger/          # 日志初始化
├── params/          # 上下文参数管理
├── service/         # 业务逻辑层
├── tools/           # 工具函数和包装器
└── utils/           # 工具函数（LLM 调用等）
```

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

## Development Notes

- 项目使用 CloudWeGo Eino 框架的 Agent Development Kit (ADK)
- 所有 LLM 调用通过 `utils.NewChatModel` 创建，支持配置化的 max_tokens、temperature 等
- 任务默认 24 小时过期，每小时清理一次过期任务
- 日志使用 uber-go/zap，支持文件轮转
- Excel 文件使用 excelize/v2 库处理
