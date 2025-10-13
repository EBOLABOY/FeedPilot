#!/bin/bash

# FeedPilot Python直接部署脚本（不使用Docker）

set -e

echo "========================================="
echo "  FeedPilot Python 直接部署"
echo "========================================="
echo ""

# 检查Python版本
if ! command -v python3 &> /dev/null; then
    echo "❌ 错误: Python3 未安装"
    echo "安装命令: sudo apt install python3 python3-venv python3-pip -y"
    exit 1
fi

PYTHON_VERSION=$(python3 --version | awk '{print $2}')
echo "✅ Python版本: $PYTHON_VERSION"
echo ""

# 创建虚拟环境
if [ ! -d "venv" ]; then
    echo "📦 创建Python虚拟环境..."
    python3 -m venv venv
    echo "✅ 虚拟环境创建完成"
else
    echo "✅ 虚拟环境已存在"
fi
echo ""

# 激活虚拟环境
echo "🔄 激活虚拟环境..."
source venv/bin/activate

# 安装依赖
echo "📥 安装Python依赖..."
pip install --upgrade pip -q
pip install -r requirements.txt -q
pip install openai>=1.0.0 -q
echo "✅ 依赖安装完成"
echo ""

# 检查环境变量配置
if [ ! -f .env ]; then
    echo "📝 创建环境变量配置..."
    if [ -f .env.docker.example ]; then
        cp .env.docker.example .env
    elif [ -f .env.example ]; then
        cp .env.example .env
    fi
    echo ""
    echo "⚠️  请编辑 .env 文件，填入你的配置："
    echo "   nano .env"
    echo ""
    echo "必填项："
    echo "  - AI_API_KEY=你的API密钥"
    echo ""
    read -p "是否现在编辑配置文件？(y/n) " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ${EDITOR:-nano} .env
    else
        echo "请手动编辑 .env 文件后重新运行"
        exit 0
    fi
fi
echo "✅ 环境变量配置已准备"
echo ""

# 创建必要目录
mkdir -p data logs config
echo "✅ 数据目录创建完成"
echo ""

# 测试运行
echo "🧪 测试运行..."
python main.py --test

if [ $? -eq 0 ]; then
    echo ""
    echo "========================================="
    echo "  ✅ 部署成功！"
    echo "========================================="
    echo ""
    echo "启动方式："
    echo ""
    echo "1. 前台运行（调试）："
    echo "   source venv/bin/activate"
    echo "   python main.py"
    echo ""
    echo "2. 执行一次："
    echo "   source venv/bin/activate"
    echo "   python main.py --once"
    echo ""
    echo "3. 后台运行（生产）："
    echo "   nohup ./venv/bin/python main.py > logs/nohup.log 2>&1 &"
    echo ""
    echo "4. 配置systemd服务（推荐）："
    echo "   sudo bash scripts/setup-systemd.sh"
    echo ""
else
    echo ""
    echo "❌ 测试失败，请检查配置"
    echo "查看日志: tail -f logs/app.log"
fi
