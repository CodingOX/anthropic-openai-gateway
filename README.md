# Anthropic-OpenAI Gateway

格式转换网关，将 Anthropic Messages API 请求转换为 OpenAI Chat Completions API 调用，并将响应还原为 Anthropic 格式。主要用于让 **Claude Code** 等 Anthropic 生态工具透明地使用 OpenAI 模型。

## 工作原理

```
Claude Code                    Gateway                       OpenAI API
   │                             │                              │
   │  POST /v1/messages          │                              │
   │  (Anthropic 格式)           │                              │
   ├────────────────────────────►│                              │
   │                             │  model 在转换列表？           │
   │                             │  ├─ 是 → 转换请求             │
   │                             │  │  POST /chat/completions    │
   │                             │  │  (OpenAI 格式)             │
   │                             │  ├──────────────────────────►│
   │                             │  │                            │
   │                             │  │  ◄─── SSE/JSON 响应 ──────│
   │                             │  │  转换响应为 Anthropic 格式  │
   │  ◄─── Anthropic 格式响应 ───│  │                            │
   │                             │  └─ 否 → 透传到 Anthropic API │
   │                             │     POST /messages            │
   │                             │     (原样转发)                 │
   │                             │  ┌───────────────────────────►│
   │                             │  │        Anthropic API       │
   │                             │  ◄────────────────────────────│
   │  ◄─── Anthropic 格式响应 ───│     (原样返回)                 │
```

## 快速开始

### 环境要求

- Go 1.23+
- OpenAI API Key
- Anthropic API Key（仅透传模型需要）

### 启动

```bash
# 1. 准备环境变量文件
cp .env.example .env.gateway

# 2. 编辑其中的 OPENAI_API_KEY 等配置
$EDITOR .env.gateway

# 3. 导出环境变量并启动网关（默认 127.0.0.1:3456）
set -a
source ./.env.gateway
set +a
go run cmd/gateway/main.go
```

### 配置 Claude Code

在新终端中设置环境变量：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
claude
```

## 配置

### 环境变量

网关运行时只读取环境变量；仓库根目录的 [.env.example](.env.example) 提供了一份模板。

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `OPENCODE_API_KEY` | 上游 opencode go API 密钥 | — |
| `BASE_URL` | 上游基础端点 | `https://api.openai.com/v1` |
| `LISTEN_PORT` | 监听端口 | `3456` |
| `NON_STREAM_TIMEOUT_MS` | 非流式请求总超时（毫秒） | `120000` |
| `TIMEOUT_MS` | 兼容旧配置的别名；仅在未设置 `NON_STREAM_TIMEOUT_MS` 时生效 | — |
| `MODELS_NEED_TRANSFORMATION` | 需转换模型列表（逗号分隔） | `gpt-4.1,gpt-4o,gpt-4o-mini,gpt-4.1-mini,gpt-4.1-nano,gpt-5,o3,o3-mini,o4-mini` |

> `NON_STREAM_TIMEOUT_MS` 只作用于非流式请求。
> 流式请求不设置总时长上限，会持续到上游结束、客户端断开或网络错误为止。
> `MODELS_NEED_TRANSFORMATION` 使用逗号分隔，解析时会自动去掉首尾空白。

## 格式转换映射

### 请求转换

| Anthropic | OpenAI |
|-----------|--------|
| `messages[].role` | `messages[].role` |
| `content` (string/array) | `content` (string/array) |
| `system` (string/array) | 首条 `system`/`developer` 消息 |
| `tools[].name` | `tools[].function.name` |
| `tools[].input_schema` | `tools[].function.parameters` |
| `tool_choice` (auto/any/tool) | `tool_choice` (auto/required/named) |
| `max_tokens` | `max_tokens` |
| `stop_sequences` | `stop` |
| `stream` | `stream` |

### 响应转换

| OpenAI | Anthropic |
|--------|-----------|
| `choices[0].message.content` | `content[].text` |
| `choices[0].message.tool_calls` | `content[].tool_use` |
| `finish_reason: "stop"` | `stop_reason: "end_turn"` |
| `finish_reason: "length"` | `stop_reason: "max_tokens"` |
| `finish_reason: "tool_calls"` | `stop_reason: "tool_use"` |
| `usage.prompt_tokens` | `usage.input_tokens` |
| `usage.completion_tokens` | `usage.output_tokens` |

### 流式转换

| OpenAI SSE | Anthropic SSE |
|-----------|---------------|
| `delta.content` | `content_block_delta` (text_delta) |
| `delta.tool_calls` | `content_block_start` → `content_block_delta` (input_json_delta) |
| `finish_reason` | `message_delta` (含 stop_reason) |
| `[DONE]` | `message_stop` |

## 健康检查

```bash
curl http://127.0.0.1:3456/health
# {"status":"ok"}
```

## 项目结构

```
.
├── cmd/gateway/main.go              # 入口
├── .env.example                     # 环境变量模板
├── internal/
│   ├── client/openai.go             # OpenAI HTTP 客户端
│   ├── client/anthropic.go          # Anthropic HTTP 客户端（透传）
│   ├── config/config.go             # 配置加载（默认值 + 环境变量）
│   ├── handler/messages.go          # /v1/messages HTTP 处理器
│   └── transformer/
│       ├── request.go               # Anthropic → OpenAI 请求转换
│       ├── response.go              # OpenAI → Anthropic 响应转换
│       └── stream.go                # SSE 流实时转换
└── pkg/types/
    ├── anthropic.go                 # Anthropic API 类型定义
    └── openai.go                    # OpenAI API 类型定义
```

## 已知限制

- **o-series 和 gpt-5 系列模型**：自动使用 `developer` role 替代 `system`，但部分模型可能仍有差异
- **thinking/推理**：Anthropic 的 extended thinking 功能未做特殊处理
- **缓存控制**：Anthropic 的 `cache_control` 会被忽略
