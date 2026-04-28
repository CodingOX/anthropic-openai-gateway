# 日志系统增强

## 概览
为网关添加了详细的日志记录功能，方便开发者和运维人员调试和监控系统。

## 日志级别和标签

日志使用以下标签进行分类：

| 标签 | 说明 | 示例 |
|------|------|------|
| `[MAIN]` | 主程序启动/关闭 | 启动成功、服务初始化 |
| `[CONFIG]` | 配置加载 | 环境变量、.env文件 |
| `[OpenAI]` | OpenAI客户端请求 | 上游API调用 |
| `[Anthropic]` | Anthropic客户端请求 | API密钥注入、请求头转发 |
| `[TRANSFORMER]` | 格式转换 | Anthropic→OpenAI、OpenAI→Anthropic |
| `[STREAM]` | 流式处理 | SSE事件流处理 |

## 日志符号说明

| 符号 | 含义 |
|------|------|
| ✅ | 成功、完成 |
| ❌ | 失败、错误 |
| ⚠️ | 警告、特殊情况 |
| 📤 | 发送请求 |
| 📥 | 接收响应 |
| 🔄 | 格式转换 |
| 📝 | 信息、数据 |
| 🔗 | 连接、URL |
| 🔐 | 安全、密钥 |
| ⏱️ | 时间、超时 |
| 📦 | 包、资源 |
| 📋 | 列表、配置 |
| 🛑 | 停止、结束 |
| 📊 | 统计、指标 |
| 🔧 | 工具、配置 |
| 🌐 | 网络、服务 |
| 📡 | 监听、待命 |
| 🚀 | 启动、初始化 |
| 🏁 | 完成、终点 |

## 日志示例

### 启动日志
```
[MAIN] 📋 正在加载配置...
[CONFIG] ✅ .env 文件已加载
[CONFIG] 📌 监听端口: 127.0.0.1:3458
[CONFIG] 🔗 上游端点: https://opencode.ai/zen/go/v1
[CONFIG] 🔐 API密钥: sk-...FNG1NDQQiK (最后10位)
[MAIN] 🚀 初始化HTTP处理器...
[MAIN] 🌐 网关启动成功，监听 127.0.0.1:3458
```

### 请求处理日志（非流式）
```
[OpenAI] 📤 发送非流式请求: model=deepseek-v4-pro, messages=1, max_tokens=100
[TRANSFORMER] 🔄 开始转换 Anthropic → OpenAI
[TRANSFORMER] ✅ 消息转换完成: 1条消息
[OpenAI] 📥 收到响应: status=200, duration=4.204s
[OpenAI] ✅ 请求成功: id=xxx, finish_reason=stop, prompt_tokens=20, completion_tokens=50
[TRANSFORMER] 🔄 开始转换 OpenAI → Anthropic
[TRANSFORMER] ✅ 响应转换完成
[TRANSFORMER] 📊 用量统计: input=20, output=50, cache_read=0, cache_creation=0
```

### 请求处理日志（流式）
```
[OpenAI] 📤 发送流式请求: model=deepseek-v4-pro, messages=1, max_tokens=100
[OpenAI] 📥 流式连接已建立: status=200, duration=100ms
[STREAM] 🎬 开始流式传输: model=deepseek-v4-pro
[STREAM] 🏁 收到 [DONE] 信号
[STREAM] ✅ 流式传输完成: 共45个事件, 耗时1.234s
```

### 错误日志
```
[OpenAI] ❌ 请求失败 (4.204s): Post "https://opencode.ai/zen/go/v1/chat/completions": EOF
[TRANSFORMER] ❌ 消息转换失败: invalid message format
[STREAM] ❌ 解析SSE数据块失败: unexpected end of JSON input
```

## 关键改进点

### 1. **配置加载日志** (`internal/config/config.go`)
- 显示 .env 文件是否成功加载
- 打印所有关键配置项（端口、URL、超时等）
- API密钥隐藏最后10位以保护安全

### 2. **OpenAI客户端日志** (`internal/client/openai.go`)
- 记录请求发送（模型、消息数、max_tokens）
- 记录响应接收（状态码、耗时）
- 记录响应内容摘要（token数、finish_reason）
- 详细的错误信息

### 3. **Anthropic客户端日志** (`internal/client/anthropic.go`)
- 记录请求转发到上游
- 记录API版本选择
- 统计转发的请求头数

### 4. **格式转换日志** (`internal/transformer/request.go`, `response.go`)
- 记录转换的开始和完成
- 显示转换的关键步骤（消息、工具、token映射）
- 统计转换后的数据量

### 5. **流式处理日志** (`internal/transformer/stream.go`)
- 流式传输的开始和完成
- SSE事件计数和耗时
- 遇到 [DONE] 信号时的通知

### 6. **主程序日志** (`cmd/gateway/main.go`)
- 彩色ASCII艺术启动横幅
- 详细的初始化步骤日志
- 路由注册确认

## 调试技巧

### 快速查找特定类型的日志
```bash
# 查看所有 OpenAI 相关日志
go run cmd/gateway/main.go 2>&1 | grep "\[OpenAI\]"

# 查看所有错误日志
go run cmd/gateway/main.go 2>&1 | grep "❌"

# 查看所有转换相关日志
go run cmd/gateway/main.go 2>&1 | grep "\[TRANSFORMER\]"
```

### 保存日志到文件
```bash
go run cmd/gateway/main.go > gateway.log 2>&1
```

### 监控日志（实时）
```bash
# macOS/Linux
go run cmd/gateway/main.go 2>&1 | tail -f

# 或使用 tee 同时输出到文件
go run cmd/gateway/main.go 2>&1 | tee gateway.log
```

## 性能影响

- ✅ 日志仅在关键流程节点输出
- ✅ 没有额外的 I/O 阻塞（使用标准的 log 包）
- ✅ 敏感信息（API密钥）已隐藏
- ⚠️ 如果日志输出到文件，可能影响性能（建议使用专业日志库升级未来版本）

## 未来改进方向

可以考虑集成专业日志库（如 `logrus`, `zap`）来实现：
- [ ] 日志级别控制（DEBUG, INFO, WARN, ERROR）
- [ ] 结构化日志输出（JSON格式）
- [ ] 日志采样和聚合
- [ ] 异步日志写入
- [ ] 日志级别动态调整
