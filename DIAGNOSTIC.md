# EOF 错误诊断指南

## 问题描述
网关收到请求后，向上游 opencode.ai 转发时收到 `EOF` 错误：
```
cause="Post \"https://opencode.ai/zen/go/v1/chat/completions\": EOF"
```

## 可能原因

### 1. API 密钥无效/过期
- 服务器拒绝连接
- 检查：`OPENCODE_API_KEY` 是否正确

### 2. API 端点错误
- BASE_URL 可能不正确
- 网关拼接的完整 URL：`{BASE_URL}/chat/completions`

### 3. 网络问题
- DNS 解析失败
- 防火墙阻止
- 代理问题

### 4. 请求格式不兼容
- opencode.ai 可能不完全兼容 OpenAI 格式
- 某些字段可能不支持

## 诊断步骤

### 方式 1: 使用诊断脚本（推荐）

```bash
cd /Users/alistar/code-all/ai/anthropic-openai-gateway
./test-api.sh
```

这个脚本会：
- ✅ 测试直接请求上游 API（跳过网关）
- ✅ 测试通过网关的请求
- ✅ 对比两者的结果

### 方式 2: 手动测试

**步骤 1：直接测试上游 API**
```bash
curl -X POST https://opencode.ai/zen/go/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-dYQocHJy8o1Q7oG90FpMjhUOn8az9JOgWEHO1J3GfogKGaZht44p9iFNG1NDQQiK" \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 50,
    "messages": [
      {
        "role": "user",
        "content": "hi"
      }
    ]
  }' -v
```

预期结果：
- ✅ 成功：返回 200 + JSON 响应
- ❌ 失败：看错误信息

**步骤 2：通过网关测试**
```bash
curl -X POST http://127.0.0.1:3458/v1/messages \
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
  }' -v
```

## 解决方案

### 如果上游 API 返回错误
- ❌ 原因：API 密钥、端点或请求格式不兼容
- ✅ 解决：
  - 验证 API 密钥是否有效
  - 确认端点是否正确
  - 查看 opencode.ai 的 API 文档

### 如果上游 API 正常，但网关失败
- ❌ 原因：网关转换逻辑有问题
- ✅ 解决：检查转换器代码

### 如果都失败
- ❌ 原因：网络/DNS 问题
- ✅ 解决：
  - `ping opencode.ai` 测试网络
  - 检查防火墙设置
  - 尝试使用代理

## 查看完整日志

启动网关时使用 verbose 模式（如果支持）：
```bash
go run cmd/gateway/main.go 2>&1 | tee gateway.log
```

然后发送请求，观察完整的错误堆栈。
