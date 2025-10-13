#!/bin/bash

# FeedPilot Docker 快速部署脚本

set -e

echo "========================================="
echo "  FeedPilot Docker 部署脚本"
echo "========================================="
echo ""

# 检查Docker是否安装
if ! command -v docker &> /dev/null; then
    echo "❌ 错误: Docker未安装"
    echo "请先安装Docker: https://docs.docker.com/get-docker/"
    exit 1
fi

# 检查Docker Compose是否安装
if ! command -v docker-compose &> /dev/null; then
    echo "❌ 错误: Docker Compose未安装"
    echo "请先安装Docker Compose: https://docs.docker.com/compose/install/"
    exit 1
fi

echo "✅ Docker环境检查通过"
echo ""

# 检查.env文件是否存在
if [ ! -f .env ]; then
    echo "📝 创建环境变量配置文件..."
    cp .env.docker.example .env
    echo ""
    echo "⚠️  请编辑 .env 文件，填入你的配置："
    echo "   vim .env"
    echo ""
    echo "必填项："
    echo "  - AI_API_KEY=你的API密钥"
    echo ""
    read -p "是否现在编辑配置文件？(y/n) " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ${EDITOR:-vim} .env
    else
        echo "请手动编辑 .env 文件后重新运行此脚本"
        exit 0
    fi
fi

echo "✅ 环境变量配置文件已准备"
echo ""

# 创建必要的目录
echo "📁 创建数据目录..."
mkdir -p data logs config
echo "✅ 目录创建完成"
echo ""

# 构建镜像
echo "🔨 构建Docker镜像..."
docker-compose build
echo "✅ 镜像构建完成"
echo ""

# 启动服务
echo "🚀 启动服务..."
docker-compose up -d
echo "✅ 服务启动完成"
echo ""

# 等待服务启动
echo "⏳ 等待服务初始化..."
sleep 5

# 检查服务状态
echo "📊 检查服务状态..."
docker-compose ps
echo ""

# 显示日志
echo "📜 最近的日志："
echo "----------------------------------------"
docker-compose logs --tail=20
echo "----------------------------------------"
echo ""

# 完成提示
echo "========================================="
echo "  ✅ 部署完成！"
echo "========================================="
echo ""
echo "常用命令："
echo "  查看日志:      docker-compose logs -f"
echo "  停止服务:      docker-compose down"
echo "  重启服务:      docker-compose restart"
echo "  查看状态:      docker-compose ps"
echo "  手动执行:      docker-compose exec feedpilot python main.py --once"
echo "  查看统计:      docker-compose exec feedpilot python main.py --stats"
echo ""
echo "详细文档请查看: DOCKER.md"
echo ""
