# 部署指南 - Anthropic-OpenAI Gateway

## 快速开始

### 1️⃣ 打包二进制文件

```bash
# 打包 x86_64（amd64）版本
./build.sh amd64

# 或打包 ARM64 版本
./build.sh arm64

# 或打包所有架构
./build.sh all
```

打包完成后，二进制文件输出在 `dist/` 目录：
```
dist/
├── gateway-linux-amd64.tar.gz
└── gateway-linux-arm64.tar.gz
```

每个压缩包内包含：`gateway`、`.env.example`、`gateway.service`。

### 2️⃣ 在服务器上部署

#### 步骤 A：上传文件到服务器

```bash
# 假设服务器 IP 为 192.168.1.100，用户为 deploy
scp dist/gateway-linux-amd64.tar.gz deploy@192.168.1.100:/tmp/

# 或使用其他工具（如 rsync）上传
```

#### 步骤 B：登录服务器并安装

```bash
# 1. 创建运行用户和目录
sudo useradd -r -s /bin/false gateway 2>/dev/null || true
sudo mkdir -p /opt/gateway /etc/gateway
sudo mkdir -p /var/log/gateway

# 2. 解压二进制文件
cd /tmp
tar -xzf gateway-linux-amd64.tar.gz
sudo mv gateway-linux-amd64/gateway /opt/gateway/gateway
sudo chmod +x /opt/gateway/gateway

# 3. 环境变量文件
sudo cp gateway-linux-amd64/.env.example /etc/gateway/gateway.env
sudo chown -R gateway:gateway /etc/gateway /opt/gateway /var/log/gateway
sudo chmod 600 /etc/gateway/gateway.env
```

### 3️⃣ 编辑环境变量文件

```bash
# 编辑 /etc/gateway/gateway.env
sudo vim /etc/gateway/gateway.env
```

关键配置项：
```bash
LISTEN_HOST=0.0.0.0
LISTEN_PORT=3456
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-xxxxx
OPENAI_TIMEOUT_MS=120000
ANTHROPIC_BASE_URL=https://api.anthropic.com/v1
ANTHROPIC_API_KEY=sk-ant-xxxxx
ANTHROPIC_TIMEOUT_MS=120000
MODELS_NEED_TRANSFORMATION=gpt-4.1,gpt-4o,gpt-4o-mini,gpt-4.1-mini,gpt-4.1-nano,gpt-5,o3,o3-mini,o4-mini
```

**⚠️ 安全提示**：
- 把敏感配置只放在 `/etc/gateway/gateway.env`
- 环境变量文件权限应为 600（只有 gateway 用户可读）

### 4️⃣ 使用 systemd 管理服务

#### 安装 systemd 服务文件

```bash
# 复制服务文件
sudo cp /tmp/gateway-linux-amd64/gateway.service /etc/systemd/system/

# 重新加载 systemd 配置
sudo systemctl daemon-reload
```

#### 启动和管理服务

```bash
# 启动服务
sudo systemctl start gateway

# 设置开机自启
sudo systemctl enable gateway

# 查看服务状态
sudo systemctl status gateway

# 实时查看日志
sudo journalctl -u gateway -f

# 重启服务
sudo systemctl restart gateway

# 停止服务
sudo systemctl stop gateway
```

### 5️⃣ 验证服务是否运行

```bash
# 检查健康端点（假设服务在 localhost:3456）
curl http://localhost:3456/health

# 预期响应
# {"status":"ok"}
```

---

## 进阶配置

### 环境变量文件

服务启动时推荐通过 `/etc/gateway/gateway.env` 统一提供配置；`gateway.service` 已通过 `EnvironmentFile` 自动加载：

```bash
OPENAI_API_KEY=sk-xxxxx
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_TIMEOUT_MS=180000
ANTHROPIC_API_KEY=sk-ant-xxxxx
ANTHROPIC_BASE_URL=https://api.anthropic.com/v1
ANTHROPIC_TIMEOUT_MS=180000
LISTEN_HOST=0.0.0.0
LISTEN_PORT=8080
MODELS_NEED_TRANSFORMATION=gpt-4.1,gpt-4o,gpt-4o-mini,gpt-4.1-mini,gpt-4.1-nano,gpt-5,o3,o3-mini,o4-mini
```

然后重新加载：
```bash
sudo systemctl daemon-reload
sudo systemctl restart gateway
```

### 反向代理配置（Nginx）

如果你想让网关在 Nginx 后面运行：

```nginx
upstream gateway {
    server 127.0.0.1:3456;
}

server {
    listen 80;
    server_name your-domain.com;

    location /v1/ {
        proxy_pass http://gateway;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # 超时配置（与 OpenAI API 超时同步）
        proxy_connect_timeout 120s;
        proxy_send_timeout 120s;
        proxy_read_timeout 120s;
    }

    location /health {
        proxy_pass http://gateway;
    }
}
```

### 日志轮转

编辑 `/etc/logrotate.d/gateway`：

```
/var/log/gateway/*.log {
    daily
    rotate 7
    compress
    delaycompress
    notifempty
    missingok
    create 0644 gateway gateway
    postrotate
        /bin/systemctl reload gateway > /dev/null 2>&1 || true
    endscript
}
```

---

## 故障排查

### 查看日志

```bash
# 查看最后 50 行日志
sudo journalctl -u gateway -n 50

# 查看实时日志
sudo journalctl -u gateway -f

# 查看错误
sudo journalctl -u gateway -p err
```

### 常见问题

**Q1：服务无法启动**
```bash
# 检查 systemd 实际加载的环境变量
sudo systemctl show gateway --property=Environment

# 检查权限
sudo -u gateway /opt/gateway/gateway  # 手动运行测试
```

**Q2：端口已被占用**
```bash
# 查看占用端口的进程
sudo lsof -i :3456

# 修改监听端口
sudo vim /etc/gateway/gateway.env
sudo systemctl restart gateway
```

**Q3：OpenAI API 超时**
```bash
# 增加超时时间
# 编辑 /etc/gateway/gateway.env，修改 OPENAI_TIMEOUT_MS 字段
```

---

## 监控和维护

### 简单的健康检查脚本

创建 `/opt/gateway/health-check.sh`：

```bash
#!/bin/bash

GATEWAY_URL="http://localhost:3456/health"
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" ${GATEWAY_URL})

if [ "${RESPONSE}" = "200" ]; then
    echo "✅ Gateway 运行正常"
    exit 0
else
    echo "❌ Gateway 异常，HTTP Code: ${RESPONSE}"
    sudo systemctl restart gateway
    exit 1
fi
```

使用 cron 定期检查：
```bash
# 每 5 分钟检查一次
*/5 * * * * /opt/gateway/health-check.sh
```

---

## 升级步骤

```bash
# 1. 在本地重新编译
./build.sh amd64

# 2. 上传新版本
scp dist/gateway-linux-amd64.tar.gz deploy@192.168.1.100:/tmp/

# 3. 在服务器上更新
ssh deploy@192.168.1.100
cd /tmp
tar -xzf gateway-linux-amd64.tar.gz
sudo systemctl stop gateway
sudo mv gateway-linux-amd64/gateway /opt/gateway/gateway
sudo chmod +x /opt/gateway/gateway
sudo systemctl start gateway

# 4. 验证
sudo systemctl status gateway
curl http://localhost:3456/health
```

---

## 总结

| 步骤 | 命令 |
|------|------|
| **打包** | `./build.sh amd64` |
| **上传** | `scp dist/gateway-linux-amd64.tar.gz user@server:/tmp/` |
| **部署** | 见「步骤 B」 |
| **启动** | `sudo systemctl start gateway` |
| **验证** | `curl http://localhost:3456/health` |
| **查看日志** | `sudo journalctl -u gateway -f` |

祝你部署顺利！🚀

---

# Docker 部署指南

## 快速开始（Docker Compose）

### 最简单的方式

```bash
# 1. 创建 .env 文件（配置 API Key）
cp .env.example .env
vim .env

# 2. 启动容器
docker-compose up -d

# 3. 验证
curl http://localhost:3456/health
```

### 查看日志

```bash
docker-compose logs -f gateway
```

### 停止容器

```bash
docker-compose down
```

---

## Docker 原理

### 文件说明

| 文件 | 用途 |
|------|------|
| `Dockerfile` | 定义 Docker 镜像构建过程（多阶段构建） |
| `docker-compose.yml` | 容器编排配置 |
| `.dockerignore` | 构建时忽略的文件 |
| `.env.example` | 环境变量模板 |

### 多阶段构建优势

- **构建阶段（Build）**：基于 `golang:1.23-alpine`，编译源代码
- **运行阶段（Runtime）**：基于 `alpine:3.19`，只包含编译后的二进制文件
- **结果**：镜像大小仅 ~30MB，而不是 600MB+

---

## 部署到服务器

### 方案 1：使用 Docker Compose（推荐）

#### 步骤 1：准备

```bash
# 1. 上传项目文件到服务器
scp -r . deploy@your-server:/home/deploy/gateway/

# 2. 登录服务器
ssh deploy@your-server
cd gateway

# 3. 创建 .env 文件
cp .env.example .env
nano .env  # 编辑，填入你的 OpenAI API Key
```

#### 步骤 2：构建镜像

```bash
# 首次部署时构建镜像
docker-compose build

# 或直接启动（自动构建）
docker-compose up -d
```

#### 步骤 3：验证和管理

```bash
# 查看容器状态
docker-compose ps

# 查看日志
docker-compose logs -f gateway

# 重启容器
docker-compose restart gateway

# 停止容器
docker-compose stop gateway

# 启动容器
docker-compose start gateway
```

### 方案 2：使用 Docker 命令行

如果不想用 Compose，也可以手动运行：

```bash
# 1. 构建镜像
docker build -t anthropic-openai-gateway:latest .

# 2. 创建运行容器
docker run -d \
  --name gateway \
  -p 3456:3456 \
  -e OPENAI_API_KEY="sk-xxxxx" \
  -e OPENAI_BASE_URL="https://api.openai.com/v1" \
  -e LISTEN_HOST="0.0.0.0" \
  -e LISTEN_PORT="3456" \
  --restart unless-stopped \
  anthropic-openai-gateway:latest

# 3. 查看日志
docker logs -f gateway

# 4. 停止和删除
docker stop gateway
docker rm gateway
```

## 反向代理配置（Nginx + Docker）

### 使用 Docker Compose 增强配置

创建完整的 `docker-compose.yml`，包含 Nginx：

```yaml
version: '3.8'

services:
  gateway:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: anthropic-openai-gateway
    restart: unless-stopped
    environment:
      OPENAI_API_KEY: "${OPENAI_API_KEY}"
      LISTEN_HOST: "0.0.0.0"
      LISTEN_PORT: "3456"
    networks:
      - gateway-network
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:3456/health"]
      interval: 30s
      timeout: 5s
      retries: 3

  nginx:
    image: nginx:alpine
    container_name: gateway-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      # - ./ssl/cert.pem:/etc/nginx/ssl/cert.pem:ro
      # - ./ssl/key.pem:/etc/nginx/ssl/key.pem:ro
    depends_on:
      - gateway
    networks:
      - gateway-network

networks:
  gateway-network:
    driver: bridge
```

### Nginx 配置文件（`nginx.conf`）

```nginx
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';

    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    keepalive_timeout 65;
    gzip on;

    upstream gateway {
        server gateway:3456;
    }

    server {
        listen 80;
        server_name _;

        location /v1/ {
            proxy_pass http://gateway;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            
            proxy_connect_timeout 120s;
            proxy_send_timeout 120s;
            proxy_read_timeout 120s;
        }

        location /health {
            proxy_pass http://gateway;
        }
    }
}
```

### 启动

```bash
docker-compose up -d
```

访问：`http://your-domain.com/v1/messages`

---

## 更新镜像版本

### 本地更新

```bash
# 1. 拉取最新代码
git pull origin main

# 2. 重新构建镜像
docker-compose build

# 3. 重新启动容器（会自动使用新镜像）
docker-compose up -d

# 4. 验证
docker-compose logs -f gateway
curl http://localhost:3456/health
```

### 清理旧镜像

```bash
# 查看镜像
docker images

# 删除旧镜像
docker rmi <image_id>

# 清理未使用的镜像和容器
docker system prune -a
```

---

## 监控和日志

### 查看实时日志

```bash
docker-compose logs -f gateway
```

### 导出日志

```bash
docker-compose logs gateway > gateway.log
```

### 配置日志轮转（Docker）

在 `docker-compose.yml` 中添加：

```yaml
services:
  gateway:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"      # 单个日志文件最大 10MB
        max-file: "3"        # 最多保留 3 个日志文件
```

---

## 故障排查

### 容器无法启动

```bash
# 查看错误日志
docker-compose logs gateway

# 检查是否端口被占用
lsof -i :3456
```

### 无法连接到 OpenAI

```bash
# 检查环境变量是否正确设置
docker-compose exec gateway env | grep OPENAI

# 测试网络连接
docker-compose exec gateway wget -O - https://api.openai.com/v1/models \
  -H "Authorization: Bearer sk-xxxxx"
```

### 健康检查失败

```bash
# 进入容器调试
docker-compose exec gateway sh

# 手动验证健康端点
wget -q -O - http://localhost:3456/health
```

---

## 对比：二进制 vs Docker

| 特性 | 二进制 | Docker |
|------|-------|--------|
| **部署复杂度** | 中等（需要 systemd） | 简单（Compose） |
| **隔离性** | 无 | 完全隔离 |
| **镜像大小** | N/A | ~30MB |
| **启动速度** | 秒级 | 秒级 |
| **升级** | 手动替换二进制 | `docker-compose build && up` |
| **多版本** | 难 | 容易（多个标签） |
| **生产推荐** | ✅ | ✅✅✅ |

---

## 总结

| 步骤 | 命令 |
|------|------|
| **配置** | `cp .env.example .env && vim .env` |
| **启动** | `docker-compose up -d` |
| **验证** | `curl http://localhost:3456/health` |
| **查看日志** | `docker-compose logs -f gateway` |
| **停止** | `docker-compose down` |
| **更新** | `docker-compose build && up -d` |

享受 Docker 部署的便利！🐳
