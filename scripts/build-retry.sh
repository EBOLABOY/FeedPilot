#!/bin/bash

# Docker构建重试脚本 - 自动重试直到成功

MAX_RETRIES=5
RETRY_DELAY=60  # 秒

echo "========================================="
echo "  Docker 自动重试构建脚本"
echo "========================================="
echo ""

for i in $(seq 1 $MAX_RETRIES); do
    echo "尝试 $i/$MAX_RETRIES: 构建Docker镜像..."

    docker-compose build

    if [ $? -eq 0 ]; then
        echo ""
        echo "✅ 构建成功！"
        exit 0
    else
        if [ $i -lt $MAX_RETRIES ]; then
            echo ""
            echo "⏳ 构建失败，等待 $RETRY_DELAY 秒后重试..."
            sleep $RETRY_DELAY
        fi
    fi
done

echo ""
echo "❌ 已达到最大重试次数 ($MAX_RETRIES)，构建失败"
echo ""
echo "建议："
echo "  1. 登录Docker Hub: bash scripts/docker-login.sh"
echo "  2. 等待一段时间后重试（速率限制会在6小时后重置）"
echo "  3. 使用已有镜像: docker pull python:3.11-slim"
exit 1
