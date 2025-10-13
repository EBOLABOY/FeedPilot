#!/bin/bash

# Docker镜像站部署脚本 - 解决Docker Hub速率限制

set -e

echo "========================================="
echo "  FeedPilot Docker 镜像站部署"
echo "========================================="
echo ""

# 检查Docker
if ! command -v docker &> /dev/null; then
    echo "❌ 错误: Docker未安装"
    echo "安装命令: curl -fsSL https://get.docker.com | sh"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "❌ 错误: Docker Compose未安装"
    exit 1
fi

echo "✅ Docker环境检查通过"
echo ""

# 检查.env文件
if [ ! -f .env ]; then
    echo "📝 创建环境变量配置..."
    cp .env.docker.example .env
    echo ""
    echo "⚠️  请编辑 .env 文件，填入API密钥"
    echo "   nano .env"
    echo ""
    read -p "是否现在编辑？(y/n) " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ${EDITOR:-nano} .env
    else
        echo "请手动编辑 .env 文件后重新运行"
        exit 0
    fi
fi

echo "✅ 环境配置已准备"
echo ""

# 创建目录
mkdir -p data logs config
echo "✅ 数据目录创建完成"
echo ""

# 构建镜像（使用镜像站）
echo "🔨 从镜像站构建Docker镜像..."
echo "使用: 阿里云镜像站 (registry.cn-hangzhou.aliyuncs.com)"
echo ""

docker-compose -f docker-compose.mirror.yml build

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ 镜像构建成功"
    echo ""

    # 启动服务
    echo "🚀 启动服务..."
    docker-compose -f docker-compose.mirror.yml up -d

    echo ""
    echo "⏳ 等待服务初始化..."
    sleep 5

    # 检查状态
    echo "📊 服务状态："
    docker-compose -f docker-compose.mirror.yml ps
    echo ""

    echo "📜 最近日志："
    docker-compose -f docker-compose.mirror.yml logs --tail=20
    echo ""

    echo "========================================="
    echo "  ✅ 部署完成！"
    echo "========================================="
    echo ""
    echo "常用命令："
    echo "  查看日志: docker-compose -f docker-compose.mirror.yml logs -f"
    echo "  停止服务: docker-compose -f docker-compose.mirror.yml down"
    echo "  重启服务: docker-compose -f docker-compose.mirror.yml restart"
    echo ""
else
    echo ""
    echo "❌ 镜像构建失败"
    echo ""
    echo "可能的原因："
    echo "  1. 阿里云镜像站连接问题"
    echo "  2. 网络问题"
    echo ""
    echo "替代方案："
    echo "  1. Python直接部署: bash scripts/deploy-python.sh"
    echo "  2. 登录Docker Hub: docker login && docker-compose build"
    exit 1
fi
