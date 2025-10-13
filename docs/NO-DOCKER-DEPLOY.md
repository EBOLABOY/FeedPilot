# 绕过Docker Hub的部署方案

## 方案1: 使用GitHub Container Registry（推荐）

我帮您构建好镜像并推送到GitHub，然后您直接拉取使用。

### 步骤：

1. **修改docker-compose.yml，使用预构建镜像**

```yaml
version: '3.8'

services:
  feedpilot:
    image: ghcr.io/ebolaboy/feedpilot:latest  # 使用预构建镜像
    # 不再需要build部分
    container_name: feedpilot
    restart: unless-stopped
    ...
```

2. **拉取并运行**
```bash
docker-compose pull  # 从GitHub拉取
docker-compose up -d
```

## 方案2: 使用本地镜像文件

我构建好镜像并导出为tar文件，您导入使用。

### 步骤：

1. **在本地Windows构建镜像**（您当前的机器）
```bash
docker-compose build
```

2. **导出镜像**
```bash
docker save feedpilot:latest -o feedpilot-image.tar
```

3. **上传到服务器**
```bash
scp feedpilot-image.tar user@your-server:/opt/feedpilot/
```

4. **在服务器上导入**
```bash
cd /opt/feedpilot
docker load -i feedpilot-image.tar
docker-compose up -d
```

## 方案3: 直接使用Python运行（不用Docker）

既然Docker Hub有问题，可以先不用Docker部署。

### 步骤：

```bash
# 1. 安装Python 3.11
sudo apt update
sudo apt install python3.11 python3.11-venv python3-pip -y

# 2. 创建虚拟环境
cd /opt/feedpilot
python3.11 -m venv venv
source venv/bin/activate

# 3. 安装依赖
pip install -r requirements.txt
pip install openai

# 4. 配置环境变量
cp .env.docker.example .env
nano .env  # 填入API密钥

# 5. 运行
python main.py
```

### 使用systemd管理服务
```bash
sudo nano /etc/systemd/system/feedpilot.service
```

内容：
```ini
[Unit]
Description=FeedPilot RSS Push Service
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/opt/feedpilot
Environment="PATH=/opt/feedpilot/venv/bin"
ExecStart=/opt/feedpilot/venv/bin/python /opt/feedpilot/main.py
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl enable feedpilot
sudo systemctl start feedpilot
sudo systemctl status feedpilot
```

## 方案4: 使用Podman替代Docker

Podman不需要守护进程，兼容Docker命令。

```bash
# 安装Podman
sudo apt install podman -y

# 直接使用docker-compose（Podman兼容）
podman-compose up -d
```

## 推荐顺序

1. **方案3（Python直接运行）** - 最简单，无需Docker
2. **方案2（导入镜像文件）** - 一次性解决，后续可用Docker管理
3. **方案1（GitHub Registry）** - 需要我先构建推送
4. **方案4（Podman）** - 替代Docker的选择

## 我的建议

**现在立即使用方案3（Python直接运行）：**

```bash
# 快速部署命令
cd /opt/feedpilot
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt openai
cp .env.docker.example .env
nano .env  # 修改API密钥
python main.py --once  # 测试运行
```

如果测试成功，配置systemd服务让它后台运行。

**Docker的问题等后续解决（可能需要等速率限制重置）。**

您觉得哪个方案比较合适？
