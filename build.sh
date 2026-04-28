#!/bin/bash

# 打包脚本：支持多个平台和架构的编译
# 用法：./build.sh [linux|windows|macos|all] [amd64|arm64|all]
# 示例：
#   ./build.sh linux amd64       # 构建 Linux x86_64
#   ./build.sh macos arm64       # 构建 macOS ARM64
#   ./build.sh all all           # 构建所有平台和架构

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${SCRIPT_DIR}/dist"
BINARY_NAME="gateway"
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')

# 默认参数
TARGET_OS="${1:-linux}"
TARGET_ARCH="${2:-amd64}"

# 支持的 OS 和 ARCH 组合
declare -a SUPPORTED_OS=("linux" "windows" "macos")
declare -a SUPPORTED_ARCH=("amd64" "arm64")

# 清理旧的构建输出
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"

# 构建函数
build_binary() {
    local os=$1
    local arch=$2
    
    # 映射 macos 到 darwin（Go 的 GOOS 值）
    local goos="${os}"
    [ "$os" = "macos" ] && goos="darwin"
    
    # 确定二进制后缀
    local binary_suffix=""
    [ "$os" = "windows" ] && binary_suffix=".exe"
    
    local package_name="${BINARY_NAME}-${os}-${arch}"
    local package_dir="${BUILD_DIR}/${package_name}"
    local output="${package_dir}/${BINARY_NAME}${binary_suffix}"
    
    echo "🔨 构建 ${os}/${arch}..."
    mkdir -p "${package_dir}"
    
    env CGO_ENABLED=0 GOOS="${goos}" GOARCH="${arch}" go build \
        -o "${output}" \
        -ldflags="-X 'main.version=${VERSION}' -X 'main.buildTime=${BUILD_TIME}'" \
        "${SCRIPT_DIR}/cmd/gateway/main.go"

    # 复制配置模板
    cp "${SCRIPT_DIR}/.env.example" "${package_dir}/.env.example"
    
    # 压缩打包（Windows 使用 zip，其他使用 tar.gz）
    if [ "$os" = "windows" ]; then
        cd "${BUILD_DIR}"
        zip -q -r "${package_name}.zip" "${package_name}"
        rm -rf "${package_dir}"
        echo "✅ 完成: ${package_name}.zip"
    else
        tar -czf "${package_dir}.tar.gz" -C "${BUILD_DIR}" "${package_name}"
        rm -rf "${package_dir}"
        echo "✅ 完成: ${package_name}.tar.gz"
    fi
}

# 构建指定的 OS 和 ARCH 组合
build_targets() {
    local os=$1
    local arch=$2
    
    if [ "$os" = "all" ]; then
        for target_os in "${SUPPORTED_OS[@]}"; do
            build_targets "$target_os" "$arch"
        done
        return
    fi
    
    if [ "$arch" = "all" ]; then
        for target_arch in "${SUPPORTED_ARCH[@]}"; do
            build_targets "$os" "$target_arch"
        done
        return
    fi
    
    # 检查有效性
    local valid_os=0
    for valid in "${SUPPORTED_OS[@]}"; do
        [ "$os" = "$valid" ] && valid_os=1
    done
    
    local valid_arch=0
    for valid in "${SUPPORTED_ARCH[@]}"; do
        [ "$arch" = "$valid" ] && valid_arch=1
    done
    
    if [ $valid_os -eq 0 ] || [ $valid_arch -eq 0 ]; then
        echo "❌ 不支持的平台或架构组合: ${os}/${arch}"
        return 1
    fi
    
    build_binary "$os" "$arch"
}

# 执行构建
build_targets "$TARGET_OS" "$TARGET_ARCH"

echo ""
echo "📦 构建完成，输出在: ${BUILD_DIR}"
ls -lh "${BUILD_DIR}"
