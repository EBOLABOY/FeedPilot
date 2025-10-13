#!/bin/bash

# 快速安装Python依赖并部署

echo "========================================="
echo "  快速修复并部署 FeedPilot"
echo "========================================="
echo ""

# 安装必要的系统包
echo "📦 安装系统依赖..."
apt update
apt install -y python3-venv python3-pip

echo "✅ 系统依赖安装完成"
echo ""

# 清理旧的虚拟环境（如果存在但损坏）
if [ -d "venv" ]; then
    echo "🧹 清理旧的虚拟环境..."
    rm -rf venv
fi

# 运行部署脚本
echo "🚀 运行部署脚本..."
bash scripts/deploy-python.sh
