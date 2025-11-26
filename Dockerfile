FROM python:3.11-slim AS base

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

# 安装系统依赖（如果后续需要可在此追加）
RUN apt-get update && apt-get install -y --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

# 复制依赖文件并安装
COPY requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

# 复制项目代码
COPY . .

# 创建运行时需要的目录
RUN mkdir -p data logs config

# 默认使用 config/app.yaml 作为配置，.env 通过外部挂载/环境配置

CMD ["sh", "-c", "mkdir -p data logs config && python main.py"]

