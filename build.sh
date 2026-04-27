#!/bin/bash

# 打包脚本：支持多个平台的编译
# 用法：./build.sh [amd64|arm64|all]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/dist"
BINARY_NAME="gateway"
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')

# 默认构建 amd64
TARGET_ARCH="${1:-amd64}"

# 清理旧的构建输出
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# 构建函数
build_binary() {
    local arch=$1
    local os="linux"
    
    # 如果需要 Windows 或 macOS，可修改此处
    if [ "$arch" = "all" ]; then
        echo "🔨 构建所有架构..."
        build_binary "amd64"
        build_binary "arm64"
        return
    fi
    
    local output="${BUILD_DIR}/${BINARY_NAME}-${os}-${arch}"
    
    echo "🔨 构建 ${os}/${arch}..."
    env CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \
        -o "${output}" \
        -ldflags="-X 'main.version=${VERSION}' -X 'main.buildTime=${BUILD_TIME}'" \
        "${SCRIPT_DIR}/cmd/gateway/main.go"
    
    # 压缩
    tar -czf "${output}.tar.gz" -C "${BUILD_DIR}" "$(basename ${output})"
    rm "${output}"
    
    echo "✅ 完成: ${output}.tar.gz"
}

# 执行构建
case "${TARGET_ARCH}" in
    amd64)
        build_binary "amd64"
        ;;
    arm64)
        build_binary "arm64"
        ;;
    all)
        build_binary "all"
        ;;
    *)
        echo "❌ 不支持的架构: ${TARGET_ARCH}"
        echo "用法: $0 [amd64|arm64|all]"
        exit 1
        ;;
esac

echo ""
echo "📦 构建完成，输出在: ${BUILD_DIR}"
ls -lh "${BUILD_DIR}"
