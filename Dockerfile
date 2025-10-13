# FeedPilot - RSS推送服务 Docker镜像
FROM python:3.11-slim

# 设置工作目录
WORKDIR /app

# 设置环境变量
ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    TZ=Asia/Shanghai

# 安装系统依赖
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        tzdata \
        ca-certificates && \
    ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 复制依赖文件
COPY requirements.txt .

# 安装Python依赖
RUN pip install --no-cache-dir -r requirements.txt && \
    pip install --no-cache-dir openai>=1.0.0

# 复制项目文件
COPY . .

# 创建必要的目录
RUN mkdir -p /app/data /app/logs && \
    chmod +x /app/main.py

# 数据卷挂载点
VOLUME ["/app/data", "/app/logs", "/app/config"]

# 健康检查
HEALTHCHECK --interval=5m --timeout=3s --start-period=30s \
    CMD python -c "import sqlite3; conn = sqlite3.connect('/app/data/pushed_items.db'); conn.close()" || exit 1

# 启动命令
CMD ["python", "-u", "main.py"]
