#!/bin/bash

# 配置FeedPilot为systemd服务

set -e

if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用sudo运行此脚本"
    echo "   sudo bash $0"
    exit 1
fi

echo "========================================="
echo "  配置FeedPilot Systemd服务"
echo "========================================="
echo ""

# 获取当前用户和工作目录
CURRENT_USER=${SUDO_USER:-$USER}
WORK_DIR=$(pwd)

echo "用户: $CURRENT_USER"
echo "工作目录: $WORK_DIR"
echo ""

# 创建systemd服务文件
SERVICE_FILE="/etc/systemd/system/feedpilot.service"

echo "创建服务文件: $SERVICE_FILE"

cat > $SERVICE_FILE <<EOF
[Unit]
Description=FeedPilot RSS Push Service
After=network.target

[Service]
Type=simple
User=$CURRENT_USER
WorkingDirectory=$WORK_DIR
Environment="PATH=$WORK_DIR/venv/bin:/usr/local/bin:/usr/bin:/bin"
ExecStart=$WORK_DIR/venv/bin/python $WORK_DIR/main.py
Restart=always
RestartSec=10

# 日志
StandardOutput=append:$WORK_DIR/logs/systemd-stdout.log
StandardError=append:$WORK_DIR/logs/systemd-stderr.log

# 安全设置
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

echo "✅ 服务文件创建完成"
echo ""

# 重载systemd
echo "重载systemd配置..."
systemctl daemon-reload

# 启用服务
echo "启用服务..."
systemctl enable feedpilot.service

# 显示服务状态
echo ""
echo "========================================="
echo "  ✅ 配置完成！"
echo "========================================="
echo ""
echo "服务管理命令："
echo "  启动服务: sudo systemctl start feedpilot"
echo "  停止服务: sudo systemctl stop feedpilot"
echo "  重启服务: sudo systemctl restart feedpilot"
echo "  查看状态: sudo systemctl status feedpilot"
echo "  查看日志: sudo journalctl -u feedpilot -f"
echo ""
echo "是否现在启动服务？"
read -p "(y/n) " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    systemctl start feedpilot
    sleep 2
    systemctl status feedpilot --no-pager
fi
