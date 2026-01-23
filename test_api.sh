#!/bin/bash

# 测试脚本
BASE_URL="http://localhost:8080/api/v1"

echo "===== 测试 Excel 处理接口 ====="
echo ""

# 1. 首先上传文件获取 task_id
echo "1. 上传测试文件..."
# 创建一个测试文件
echo '{"name":"test","value":"123"}' > /tmp/test.json

UPLOAD_RESP=$(curl -s -X POST "$BASE_URL/excel/upload" \
  -F "file=@/tmp/test.json" \
  -F "prompt=分析这个文件")

echo "上传响应: $UPLOAD_RESP"

# 提取 task_id (假设响应格式为 {"data":{"task_id":"xxx"}})
TASK_ID=$(echo $UPLOAD_RESP | grep -o '"task_id":"[^"]*' | cut -d'"' -f4)

if [ -z "$TASK_ID" ]; then
  echo "错误: 无法获取 task_id"
  echo "请先确保服务器正在运行: go run main.go"
  exit 1
fi

echo "获取到 task_id: $TASK_ID"
echo ""

# 2. 测试同步处理接口
echo "2. 测试同步处理接口 /process..."
PROCESS_RESP=$(curl -s -X POST "$BASE_URL/excel/process" \
  -H "Content-Type: application/json" \
  -d "{\"task_id\":\"$TASK_ID\",\"prompt\":\"请分析文件内容\"}")

echo "处理响应: $PROCESS_RESP"
echo ""

# 3. 测试异步处理接口
echo "3. 测试异步处理接口 /process/async..."
ASYNC_RESP=$(curl -s -X POST "$BASE_URL/excel/process/async" \
  -H "Content-Type: application/json" \
  -d "{\"task_id\":\"$TASK_ID\",\"prompt\":\"请异步分析文件内容\"}")

echo "异步处理响应: $ASYNC_RESP"
echo ""

# 4. 查询任务状态
echo "4. 查询任务状态..."
sleep 2
STATUS_RESP=$(curl -s -X GET "$BASE_URL/excel/task/$TASK_ID")
echo "状态响应: $STATUS_RESP"
echo ""

echo "===== 测试完成 ====="
