#!/bin/bash

# Docker国内镜像源配置脚本 - Ubuntu/Debian

echo "正在配置Docker国内镜像源..."

# 创建或修改daemon.json
sudo mkdir -p /etc/docker

sudo tee /etc/docker/daemon.json > /dev/null <<EOF
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com",
    "https://mirror.ccs.tencentyun.com"
  ],
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
EOF

echo "镜像源配置完成"

# 重启Docker服务
echo "重启Docker服务..."
sudo systemctl daemon-reload
sudo systemctl restart docker

echo "验证配置..."
sudo docker info | grep -A 10 "Registry Mirrors"

echo ""
echo "✅ 配置完成！现在可以重新运行 docker-compose build"
