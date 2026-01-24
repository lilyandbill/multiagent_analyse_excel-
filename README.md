# Excel Agent

一个基于 LLM 的 Excel 智能处理代理，支持通过自然语言对话来操作和分析 Excel 文件。

## 功能特点

- 📊 Excel 文件读取和处理
- 🤖 基于 LLM 的智能对话
- 📝 支持生成问题和答案
- 🔍 支持网络搜索
- 💻 支持代码执行

## 快速开始

### 1. 安装依赖

```bash
go mod tidy
```

### 2. 配置

复制配置文件模板并修改：

```bash
cp config.example.yaml config.yaml
```

编辑 `config.yaml`，填入你的 API Key：

```yaml
llm:
  model: "glm-4.7"
  api_key: "your-api-key-here"
  base_url: "https://open.bigmodel.cn/api/paas/v4/"
```

### 3. 运行

```bash
go run main.go
```

服务器将在 `http://localhost:8080` 启动。

## 项目结构

```
excel-agent/
├── agents/          # 代理逻辑
│   ├── executor/    # 执行器
│   ├── planner/     # 规划器
│   ├── replanner/   # 重规划器
│   └── report/      # 报告生成
├── api/             # API 接口
│   ├── handler/     # 处理器
│   └── router/      # 路由
├── config/          # 配置加载
├── excel/           # Excel 文件目录
│   ├── uploads/     # 上传文件
│   └── results/     # 结果文件
├── frontend/        # 前端页面
├── generic/         # 通用工具
├── logger/          # 日志模块
├── operator.go      # 操作器
├── params/          # 参数定义
├── service/         # 服务层
├── tools/           # 工具函数
└── utils/           # 工具函数
```

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
```bash
curl -X POST "http://localhost:8080/api/v1/excel/analyze" \
  -F "file=@data.xlsx" \
  -F "prompt=计算销售额总和" \
  -F "async=false"
```

**异步分析（推荐用于复杂任务）：**
```bash
curl -X POST "http://localhost:8080/api/v1/excel/analyze" \
  -F "file=@data.xlsx" \
  -F "prompt=生成数据透视表和图表" \
  -F "async=true"
```

## 配置说明

### LLM 配置

```yaml
llm:
  model: "glm-4.7"           # 模型名称
  api_key: ""                # API Key
  base_url: ""               # Base URL（可选）
  temperature: 0.7           # 温度参数
```

### Excel 配置

```yaml
excel:
  dir: "./excel"             # Excel 文件目录
  max_rows: 10000            # 最大读取行数
```

### 服务器配置

```yaml
server:
  host: "0.0.0.0"            # 监听地址
  port: 8080                 # 端口号
```

## License

MIT
