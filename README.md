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
   │                             │  └─ 否 → 直接透传（未实现）    │
```

## 快速开始

### 环境要求

- Go 1.23+
- OpenAI API Key

### 启动

```bash
# 1. 复制配置文件
cp configs/config.example.json configs/config.json

# 2. 设置 API Key
export OPENAI_API_KEY=sk-xxx

# 3. 启动网关（默认 127.0.0.1:3456）
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

### 配置文件 `configs/config.json`

```json
{
  "listen_host": "127.0.0.1",
  "listen_port": 3456,
  "openai": {
    "base_url": "https://api.openai.com/v1",
    "api_key": "sk-xxx",
    "timeout_ms": 120000
  },
  "models_need_transformation": [
    "gpt-4.1",
    "gpt-4o",
    "gpt-4o-mini",
    "gpt-4.1-mini",
    "gpt-4.1-nano",
    "gpt-5",
    "o3",
    "o3-mini",
    "o4-mini"
  ]
}
```

### 环境变量覆盖

所有配置项均可用环境变量覆盖（优先级更高）：

| 环境变量 | 对应配置 | 默认值 |
|---------|---------|--------|
| `CONFIG_FILE` | 配置文件路径 | `./configs/config.json` |
| `LISTEN_HOST` | 监听地址 | `127.0.0.1` |
| `LISTEN_PORT` | 监听端口 | `3456` |
| `OPENAI_BASE_URL` | OpenAI API 地址 | `https://api.openai.com/v1` |
| `OPENAI_API_KEY` | API 密钥 | — |
| `OPENAI_TIMEOUT_MS` | 请求超时（毫秒） | `120000` |

> **注意**：`api_key` 在 JSON 配置中使用 `${OPENAI_API_KEY}` 占位时，会从环境变量中读取实际值。

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
| `max_tokens` | `max_completion_tokens` |
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
├── configs/config.example.json      # 配置模板
├── internal/
│   ├── client/openai.go             # OpenAI HTTP 客户端
│   ├── config/config.go             # 配置加载（文件 + 环境变量覆盖）
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

- **透传模式未实现**：不在 `models_need_transformation` 列表中的模型返回 501
- **o-series 模型**：自动使用 `developer` role 替代 `system`，但部分 o 系列模型可能仍有差异
- **thinking/推理**：Anthropic 的 extended thinking 功能未做特殊处理
- **缓存控制**：Anthropic 的 `cache_control` 会被忽略
