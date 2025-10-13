#!/bin/bash

# Docker Hub 登录脚本 - 解决速率限制问题

echo "========================================="
echo "  Docker Hub 登录配置"
echo "========================================="
echo ""
echo "Docker Hub免费账户拉取限制："
echo "  - 匿名用户: 100次/6小时"
echo "  - 登录用户: 200次/6小时"
echo ""
echo "如果没有Docker Hub账户，请访问: https://hub.docker.com/signup"
echo ""

read -p "请输入Docker Hub用户名: " DOCKER_USERNAME
read -sp "请输入Docker Hub密码: " DOCKER_PASSWORD
echo ""

# 登录Docker Hub
echo ""
echo "正在登录Docker Hub..."
echo "$DOCKER_PASSWORD" | docker login -u "$DOCKER_USERNAME" --password-stdin

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ 登录成功！"
    echo ""
    echo "现在可以继续构建镜像:"
    echo "  docker-compose build"
    echo ""
else
    echo ""
    echo "❌ 登录失败，请检查用户名和密码"
    exit 1
fi
