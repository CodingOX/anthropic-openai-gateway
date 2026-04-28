#!/bin/bash

# 测试脚本：诊断网关和上游 API 连接

set -e

echo "=========================================="
echo "OpenAI Gateway 诊断测试"
echo "=========================================="

# 从 .env 读取配置
if [ -f .env ]; then
  export $(cat .env | grep -v '^#' | xargs)
  echo "✅ 已加载 .env 文件"
else
  echo "❌ 找不到 .env 文件"
  exit 1
fi

echo ""
echo "配置:"
echo "  LISTEN_PORT: ${LISTEN_PORT:-3458}"
echo "  BASE_URL: ${BASE_URL}"
echo "  OPENCODE_API_KEY: ${OPENCODE_API_KEY:0:10}..."
echo ""

# 1. 测试上游 API (跳过网关)
echo "========== 测试 1: 直接测试上游 API =========="
echo "请求: POST ${BASE_URL}/chat/completions"
echo ""

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${BASE_URL}/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${OPENCODE_API_KEY}" \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 50,
    "messages": [
      {
        "role": "user",
        "content": "hi"
      }
    ]
  }')

HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "HTTP 状态码: $HTTP_CODE"
echo "响应体:"
echo "$BODY" | head -c 500
echo ""
echo ""

if [ "$HTTP_CODE" = "200" ]; then
  echo "✅ 上游 API 连接正常"
else
  echo "⚠️  上游 API 返回: $HTTP_CODE"
  echo "响应内容:"
  echo "$BODY"
fi

# 2. 测试网关
echo ""
echo "========== 测试 2: 通过网关测试 =========="
echo "请求: POST http://127.0.0.1:${LISTEN_PORT:-3458}/v1/messages"
echo ""

RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "http://127.0.0.1:${LISTEN_PORT:-3458}/v1/messages" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 50,
    "messages": [
      {
        "role": "user",
        "content": "hi"
      }
    ]
  }')

HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "HTTP 状态码: $HTTP_CODE"
echo "响应体:"
echo "$BODY" | head -c 500
echo ""

if [ "$HTTP_CODE" = "200" ]; then
  echo "✅ 网关正常工作"
else
  echo "⚠️  网关返回: $HTTP_CODE"
  echo "完整响应:"
  echo "$BODY"
fi

echo ""
echo "=========================================="
echo "诊断完成"
echo "=========================================="
