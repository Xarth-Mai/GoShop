#!/usr/bin/env bash
set -euo pipefail

# GoShop 微服务本地冒烟调试运行脚本
# 用法:
#   ./deploy/smoke_run.sh start   - 在后台启动所有微服务并重定向日志
#   ./deploy/smoke_run.sh stop    - 停止所有后台运行的微服务
#   ./deploy/smoke_run.sh status  - 查看当前服务的运行状态与日志位置

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PID_FILE="${PROJECT_ROOT}/bin/.pids"
LOG_DIR="${PROJECT_ROOT}/logs"
CONFIG_FILE="${PROJECT_ROOT}/config.yaml"

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
export GOCACHE="/tmp/goshop-go-build"

start_services() {
    echo "=== 正在启动 GoShop 所有微服务 ==="
    mkdir -p "${LOG_DIR}"

    # 检查并自动构建二进制
    for SERVICE in "${SERVICES[@]}"; do
        if [ ! -f "${PROJECT_ROOT}/bin/${SERVICE}" ]; then
            echo "⚠️ 未找到编译的二进制文件，正在触发一键构建..."
            "${SCRIPT_DIR}/build.sh"
            break
        fi
    done

    if [ -f "${PID_FILE}" ]; then
        echo "❌ 检测到已有 PID 记录文件 ${PID_FILE}，请先执行: ./deploy/smoke_run.sh stop"
        exit 1
    fi

    touch "${PID_FILE}"

    # 优先启动 NATS JetStream 服务
    NATS_BIN=""
    if [ -f "${PROJECT_ROOT}/bin/nats-server" ]; then
        NATS_BIN="${PROJECT_ROOT}/bin/nats-server"
    elif command -v nats-server &> /dev/null; then
        NATS_BIN="nats-server"
    fi

    if [ -n "${NATS_BIN}" ]; then
        echo "🚀 启动 NATS JetStream (端口: 4222) -> 日志: logs/nats-server.log"
        nohup "${NATS_BIN}" -js > "${LOG_DIR}/nats-server.log" 2>&1 &
        PID=$!
        echo "nats-server:${PID}" >> "${PID_FILE}"
        sleep 0.5
    else
        echo "⚠️ 未找到 nats-server 二进制文件，跳过启动 NATS JetStream。"
    fi

    # 各服务的默认端口配置（对应 docs/microservices.md 规划）
    declare -A PORTS=(
        ["goshop-user-service"]="8101"
        ["goshop-product-service"]="8102"
        ["goshop-inventory-service"]="8103"
        ["goshop-promotion-service"]="8104"
        ["goshop-order-service"]="8105"
        ["goshop-payment-service"]="8106"
        ["goshop-aftersale-service"]="8107"
        ["goshop-cart-service"]="8108"
        ["goshop-scheduler-service"]="8109"
    )

    for SERVICE in "${SERVICES[@]}"; do
        PORT="${PORTS[$SERVICE]}"
        LOG_FILE="${LOG_DIR}/${SERVICE}.log"
        BIN_PATH="${PROJECT_ROOT}/bin/${SERVICE}"

        echo "🚀 启动 ${SERVICE} (端口: ${PORT}) -> 日志: logs/${SERVICE}.log"
        
        # 通过环境变量注入配置和对应端口，使用 nohup 防止终端会话结束导致进程退出
        PORT="${PORT}" GOSHOP_CONFIG="${CONFIG_FILE}" nohup "${BIN_PATH}" > "${LOG_FILE}" 2>&1 &
        PID=$!
        echo "${SERVICE}:${PID}" >> "${PID_FILE}"
        sleep 0.2
    done

    echo "✅ 所有微服务已在后台并发启动！"
    echo "使用 './deploy/smoke_run.sh status' 查看状态，'./deploy/smoke_run.sh stop' 停止服务。"
}

stop_services() {
    echo "=== 正在停止 GoShop 微服务 ==="
    if [ ! -f "${PID_FILE}" ]; then
        echo "⚠️ 未找到 PID 记录文件 ${PID_FILE}，服务可能未启动。"
        return
    fi

    while IFS= read -r line || [ -n "$line" ]; do
        if [ -z "$line" ]; then continue; fi
        SERVICE=$(echo "$line" | cut -d':' -f1)
        PID=$(echo "$line" | cut -d':' -f2)

        if kill -0 "${PID}" 2>/dev/null; then
            echo "⏹️ 正在停止 ${SERVICE} (PID: ${PID})..."
            kill "${PID}"
            
            # 等待最大 3 秒优雅退出，否则强制 kill
            for i in {1..30}; do
                if ! kill -0 "${PID}" 2>/dev/null; then
                    break
                fi
                sleep 0.1
            done
            
            if kill -0 "${PID}" 2>/dev/null; then
                echo "⚠️ 服务未响应，强制杀死进程 (PID: ${PID})..."
                kill -9 "${PID}"
            fi
        else
            echo "ℹ️ 服务 ${SERVICE} (PID: ${PID}) 已经退出。"
        fi
    done < "${PID_FILE}"

    rm -f "${PID_FILE}"
    echo "✅ 所有微服务进程已清理完成！"
}

show_status() {
    echo "=== GoShop 微服务运行状态 ==="
    if [ ! -f "${PID_FILE}" ]; then
        echo "❌ 服务未在后台运行（无 PID 记录）。"
        return
    fi

    printf "%-30s %-10s %-10s\n" "服务名称" "PID" "运行状态"
    printf "%-30s %-10s %-10s\n" "------------------------------" "----------" "----------"
    while IFS= read -r line || [ -n "$line" ]; do
        if [ -z "$line" ]; then continue; fi
        SERVICE=$(echo "$line" | cut -d':' -f1)
        PID=$(echo "$line" | cut -d':' -f2)

        if kill -0 "${PID}" 2>/dev/null; then
            STATUS="RUNNING"
        else
            STATUS="STOPPED"
        fi
        printf "%-30s %-10s %-10s\n" "${SERVICE}" "${PID}" "${STATUS}"
    done < "${PID_FILE}"
}

COMMAND="${1:-}"
case "${COMMAND}" in
    start)
        start_services
        ;;
    stop)
        stop_services
        ;;
    status)
        show_status
        ;;
    *)
        echo "用法: $0 {start|stop|status}"
        exit 1
        ;;
esac
