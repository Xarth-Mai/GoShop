#!/usr/bin/env bash
set -euo pipefail

# GoShop 微服务一键构建脚本
# 此脚本编译 cmd/ 目录下的所有微服务入口，并将二进制输出到项目根目录下的 bin/ 目录。

# 获取项目根目录绝对路径
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== 开始构建 GoShop 微服务二进制 ==="
echo "项目根目录: ${PROJECT_ROOT}"

# 创建输出 bin 目录
mkdir -p "${PROJECT_ROOT}/bin"

# 待编译的服务列表
SERVICES=(
    "goshop-user-service"
    "goshop-product-service"
    "goshop-inventory-service"
    "goshop-promotion-service"
    "goshop-order-service"
    "goshop-payment-service"
    "goshop-aftersale-service"
    "goshop-cart-service"
    "goshop-scheduler-service"
)

# 确保 GOCACHE 设置合理，避免在沙箱只读环境下报错
if [ -z "${GOCACHE:-}" ]; then
    export GOCACHE="/tmp/goshop-go-build"
    echo "未检测到 GOCACHE，已默认设置为: ${GOCACHE}"
fi

for SERVICE in "${SERVICES[@]}"; do
    SRC_DIR="${PROJECT_ROOT}/cmd/${SERVICE}"
    OUT_BIN="${PROJECT_ROOT}/bin/${SERVICE}"
    
    if [ ! -d "${SRC_DIR}" ]; then
        echo "❌ 错误: 未找到服务目录 ${SRC_DIR}"
        continue
    fi
    
    echo "👉 正在构建 ${SERVICE}..."
    (
        cd "${PROJECT_ROOT}"
        go build -ldflags="-s -w" -o "${OUT_BIN}" "./cmd/${SERVICE}"
    )
    echo "✅ 构建成功 -> ${OUT_BIN}"
done

echo "=== GoShop 所有微服务二进制构建完成 ==="
