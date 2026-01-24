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
