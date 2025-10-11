#!/bin/bash

echo "========================================"
echo "安装AI内容筛选依赖"
echo "========================================"
echo

echo "正在安装 openai 库..."
pip install openai

echo
echo "========================================"
echo "安装完成!"
echo "========================================"
echo
echo "接下来请:"
echo "1. 复制 .env.example 为 .env"
echo "2. 编辑 .env 填入你的API配置"
echo "3. 运行 python main.py --once 测试"
echo
