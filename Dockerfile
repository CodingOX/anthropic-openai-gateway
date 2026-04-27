# 多阶段构建：第一阶段编译，第二阶段运行

# Stage 1: Build
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /build

# 复制源代码
COPY . .

# 构建二进制文件（禁用 CGO 以确保静态链接）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -o gateway \
    -ldflags="-X 'main.version=$(git describe --tags --always 2>/dev/null || echo dev)' -X 'main.buildTime=$(date -u '+%Y-%m-%d %H:%M:%S')'" \
    cmd/gateway/main.go

# Stage 2: Runtime
FROM alpine:3.19

# 安装 ca-certificates（HTTPS 支持）和 tzdata（时区支持）
RUN apk add --no-cache ca-certificates tzdata

# 创建运行用户
RUN addgroup -S gateway && adduser -S -G gateway gateway

# 设置工作目录
WORKDIR /app

# 从编译阶段复制二进制文件
COPY --from=builder /build/gateway /app/gateway

# 复制配置示例（可选）
COPY --chown=gateway:gateway configs/config.example.json /app/config.example.json

# 修改权限
RUN chmod +x /app/gateway && chown -R gateway:gateway /app

# 切换到 gateway 用户
USER gateway

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD [ "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:3456/health" ] || exit 1

# 默认监听端口
EXPOSE 3456

# 启动网关
# 注意：配置文件路径应通过环境变量或卷挂载指定
ENTRYPOINT ["/app/gateway"]
