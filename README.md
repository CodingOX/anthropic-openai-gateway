# Anthropic-OpenAI Gateway

格式转换网关，将 Anthropic Messages API 请求转换为 OpenAI Chat Completions API 调用，并将响应还原为 Anthropic 格式。主要用于让 **Claude Code** 等 Anthropic 生态工具透明地使用各类大语言模型。

上游统一使用 **OpenCode-Go** 端点，网关根据模型是否在转换列表中决定走格式转换或原样透传。

## 工作原理

```
Claude Code                    Gateway                       OpenCode-Go
   │                             │                              │
   │  POST /v1/messages          │                              │
   │  (Anthropic 格式)           │                              │
   ├────────────────────────────►│                              │
   │                             │  model 在转换列表？           │
   │                             │  ├─ 是 → 格式转换             │
   │                             │  │  POST /chat/completions    │
   │                             │  │  (OpenAI 格式)             │
   │                             │  ├──────────────────────────►│
   │                             │  │                            │
   │                             │  │  ◄─── SSE/JSON 响应 ──────│
   │                             │  │  转换为 Anthropic 格式      │
   │  ◄─── Anthropic 格式响应 ───│  │                            │
   │                             │  └─ 否 → 透传                 │
   │                             │     POST /messages            │
   │                             │     (原样转发 Anthropic 格式)   │
   │                             │  ├──────────────────────────►│
   │                             │  ◄────────────────────────────│
   │  ◄─── Anthropic 格式响应 ───│     (原样返回)                 │
```

- **转换路径**：Anthropic → OpenAI 格式请求 → OpenCode-Go → OpenAI → Anthropic 格式响应
- **透传路径**：Anthropic 请求原样转发 → OpenCode-Go → Anthropic 响应原样返回

## 快速开始

### 环境要求

- Go 1.23+
- OpenCode-Go Key

### 启动

```bash
# 1. 准备环境变量文件
cp .env.example .env

# 2. 编辑其中的 OPENCODE_API_KEY 等配置
$EDITOR .env

# 3. 启动网关（默认监听 0.0.0.0:3456）
go run cmd/gateway/main.go
```

> `godotenv` 自动加载当前目录下的 `.env` 文件，也可直接 export 环境变量后启动。

### 配置 Claude Code

在新终端中设置环境变量：

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
claude
```

## 配置

### 环境变量

网关运行时读取环境变量和 `.env` 文件；仓库根目录的 [.env.example](.env.example) 提供了一份模板。

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `OPENCODE_API_KEY` | 上游 OpenCode-Go API 密钥（必需） | — |
| `BASE_URL` | 上游基础端点 | `https://api.openai.com/v1` |
| `LISTEN_PORT` | 监听端口 | `3456` |
| `NON_STREAM_TIMEOUT_MS` | 非流式请求总超时（毫秒） | `120000` |
| `TIMEOUT_MS` | 兼容旧配置的别名；仅在未设置 `NON_STREAM_TIMEOUT_MS` 时生效 | — |
| `MODELS_NEED_TRANSFORMATION` | 需转换模型列表（逗号分隔） | `deepseek-v4-pro,deepseek-v4-flash,glm-5.1,glm-5,kimi-k2.6,kimi-k2.5,qwen3.6-plus` |

> **超时**：`NON_STREAM_TIMEOUT_MS` 只作用于非流式请求。流式请求不设总时长上限，会持续到上游结束、客户端断开或网络错误为止。
>
> **模型列表**：`MODELS_NEED_TRANSFORMATION` 使用逗号分隔，解析时自动去除首尾空白。
>
> **透传模式**：不在转换列表中的模型走透传路径，网关将 Anthropic 格式请求原样转发到 `{BASE_URL}/messages`，响应不做任何修改。

## API 端点

| 端点 | 说明 |
|------|------|
| `POST /v1/messages` | 主端点，处理消息请求（流式/非流式） |
| `POST /v1/messages/count_tokens` | 基于本地 tiktoken 字典的请求 token 预估；不调用上游模型 |
| `GET /health` | 健康检查 |

## 格式转换映射

### 请求转换

| Anthropic | OpenAI |
|-----------|--------|
| `messages[].role` | `messages[].role` |
| `content` (string/array) | `content` (string/array) |
| `system` (string/array) | 首条 `system` 消息 |
| `system` (o-series/gpt-5) | 首条 `developer` 消息 |
| `tools[].name` | `tools[].function.name` |
| `tools[].input_schema` | `tools[].function.parameters` |
| `tool_choice` (auto/any/tool) | `tool_choice` (auto/required/named) |
| `max_tokens` | `max_tokens` |
| `stop_sequences` | `stop` |
| `stream` | `stream` |
| `content[].thinking` | `reasoning_content` |

### 响应转换

| OpenAI | Anthropic |
|--------|-----------|
| `choices[0].message.content` | `content[].text` |
| `choices[0].message.reasoning_content` | `content[].thinking` |
| `choices[0].message.tool_calls` | `content[].tool_use` |
| `finish_reason: "stop"` | `stop_reason: "end_turn"` |
| `finish_reason: "length"` | `stop_reason: "max_tokens"` |
| `finish_reason: "tool_calls"` | `stop_reason: "tool_use"` |
| `usage.prompt_tokens` | `usage.input_tokens` |
| `usage.completion_tokens` | `usage.output_tokens` |
| `usage.prompt_cache_hit_tokens` | `usage.cache_read_input_tokens` |
| `usage.prompt_tokens_details.cached_tokens` | `usage.cache_read_input_tokens` |
| `usage.prompt_cache_miss_tokens` | `usage.cache_creation_input_tokens` |

实际 `/v1/messages` 请求优先使用上游 OpenAI 响应中的 `usage`，非流式响应直接读取响应体 usage；流式响应会发送 `stream_options.include_usage=true`，并在最终 usage chunk 到达后写入 Anthropic `message_delta.usage`。`count_tokens` 是独立的预估接口，不能代表一次真实生成请求的最终计费用量。

### 流式转换

| OpenAI SSE | Anthropic SSE |
|-----------|---------------|
| `delta.content` | `content_block_delta` (text_delta) |
| `delta.reasoning_content` | `content_block_delta` (thinking_delta) |
| `delta.tool_calls` | `content_block_start` → `content_block_delta` (input_json_delta) |
| `finish_reason` | `message_delta` (含 stop_reason) |
| `[DONE]` | `message_delta` + `message_stop` |

## 健康检查

```bash
curl http://127.0.0.1:3456/health
# {"status":"ok"}
```

## 项目结构

```
.
├── cmd/gateway/main.go              # 入口，路由注册
├── .env.example                     # 环境变量模板
├── internal/
│   ├── client/
│   │   ├── openai.go                # OpenAI 格式 HTTP 客户端（含重试、EOF 处理）
│   │   └── anthropic.go             # Anthropic 格式透传客户端
│   ├── config/config.go             # 配置加载（默认值 → .env → 环境变量）
│   ├── handler/
│   │   ├── messages.go              # /v1/messages 处理器（路由、校验、日志）
│   │   └── recover.go               # panic 恢复中间件
│   ├── logger/logger.go             # 结构化日志（支持预设字段上下文）
│   └── transformer/
│       ├── request.go               # Anthropic → OpenAI 请求转换
│       ├── response.go              # OpenAI → Anthropic 非流式响应转换
│       └── stream.go                # SSE 流实时转换
└── pkg/types/
    ├── anthropic.go                 # Anthropic API 类型定义
    └── openai.go                    # OpenAI API 类型定义
```

## 特性

- **双模式路由**：转换列表内的模型走格式转换，之外的模型原样透传
- **流式 & 非流式**：完整支持 SSE 流式和非流式两种模式
- **非流式重试**：遇到瞬时 EOF 自动重试一次（共 2 次尝试）
- **Panic 恢复**：全局 recover 中间件，防止单个请求崩溃影响整个服务
- **结构化日志**：每条请求带唯一 `request_id`，可追踪完整生命周期
- **真实用量透传**：`/v1/messages` 优先使用上游响应 `usage` 映射 input/output/cache token
- **token 预估**：提供 `/v1/messages/count_tokens` 兼容端点，基于本地 tiktoken 字典计算请求侧 token
- **thinking/推理**：支持 Anthropic thinking 块与 OpenAI `reasoning_content` 双向转换
- **缓存用量**：透传 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`，并兼容 OpenAI 官方 `prompt_tokens_details.cached_tokens`

## 已知限制

- **Anthropic extended thinking**：基本 thinking 文本已支持转换，但 `signature`、`redacted_thinking` 等扩展字段会被忽略
- **缓存控制**：Anthropic 文本块上的 `cache_control` 会随转换请求透传；是否命中仍取决于上游模型与缓存实现
- **top_k**：Anthropic 的 `top_k` 参数目前未映射到 OpenAI 格式
- **count_tokens**：使用内嵌 tiktoken 字典做本地预估，不会在运行时下载字典，也不会调用上游模型；图片、缓存控制等 Anthropic 特有开销仍可能与官方计费存在差异
- **流式 usage**：流式请求依赖上游返回最终 usage chunk；如果连接在最终 chunk 前中断，响应可能没有最终 `message_delta.usage`
