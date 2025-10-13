# Ubuntu服务器快速部署指南

## 📋 前置要求

- Ubuntu 18.04+ (推荐 20.04 或 22.04)
- 2GB+ RAM
- 10GB+ 可用磁盘空间
- Root或sudo权限
- 服务器已联网

## 🚀 一键部署（推荐）

### 步骤1: 安装Docker和Docker Compose

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装Docker
curl -fsSL https://get.docker.com | sh

# 添加当前用户到docker组（可选，避免每次都用sudo）
sudo usermod -aG docker $USER

# 重新登录使docker组权限生效
# 或者运行: newgrp docker

# 安装Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# 验证安装
docker --version
docker-compose --version
```

### 步骤2: 下载项目

```bash
# 方式1: 使用git（推荐）
git clone <your-repo-url> /opt/feedpilot
cd /opt/feedpilot

# 方式2: 手动上传
# 在本地打包: tar -czf feedpilot.tar.gz .
# 上传到服务器: scp feedpilot.tar.gz user@server:/opt/
# 解压: cd /opt && tar -xzf feedpilot.tar.gz && mv <folder> feedpilot
```

### 步骤3: 配置环境变量

```bash
cd /opt/feedpilot

# 复制环境变量模板
cp .env.docker.example .env

# 编辑配置文件
nano .env
# 或使用vim: vim .env
```

**必须修改的配置：**
```bash
AI_API_KEY=your-actual-api-key-here
```

其他配置保持默认即可。按 `Ctrl+X`, 然后 `Y`, 然后 `Enter` 保存退出。

### 步骤3.5: 登录Docker Hub（避免速率限制）

```bash
# 登录Docker Hub以提高拉取限额
docker login

# 或使用脚本
bash scripts/docker-login.sh
```

**为什么需要登录？**
- Docker Hub限制匿名用户: 100次拉取/6小时
- 登录后提升至: 200次拉取/6小时
- 详细说明: [docs/DOCKER-RATELIMIT.md](docs/DOCKER-RATELIMIT.md)

### 步骤4: 启动服务

```bash
# 方式1: 使用快速部署脚本
bash deploy.sh

# 方式2: 手动启动
docker-compose up -d
```

### 步骤5: 验证服务

```bash
# 查看容器状态
docker-compose ps

# 查看实时日志
docker-compose logs -f

# 按 Ctrl+C 退出日志查看
```

如果看到类似以下日志，说明启动成功：
```
feedpilot | 已加载阶段1提示词
feedpilot | 已加载阶段2提示词
feedpilot | 两阶段筛选已启用
```

## 🔧 常用管理命令

### 服务管理
```bash
cd /opt/feedpilot

# 启动服务
docker-compose up -d

# 停止服务
docker-compose down

# 重启服务
docker-compose restart

# 查看状态
docker-compose ps

# 查看日志（最近100行）
docker-compose logs --tail=100

# 实时查看日志
docker-compose logs -f
```

### 功能测试
```bash
# 测试推送连接
docker-compose exec feedpilot python main.py --test

# 执行一次完整推送
docker-compose exec feedpilot python main.py --once

# 查看推送统计
docker-compose exec feedpilot python main.py --stats

# 清理30天前的记录
docker-compose exec feedpilot python main.py --cleanup 30
```

### 配置修改
```bash
# 修改环境变量
nano .env
docker-compose restart

# 修改应用配置
nano config/app.yaml
docker-compose restart

# 修改提示词
nano 阶段1提示词.md
nano 阶段2提示词.md
docker-compose restart
```

## 📊 监控与维护

### 查看资源使用
```bash
# 查看容器资源使用
docker stats feedpilot

# 查看磁盘使用
du -sh /opt/feedpilot/{data,logs}

# 查看系统资源
htop  # 需要先安装: sudo apt install htop
```

### 日志管理
```bash
# 查看日志文件大小
ls -lh logs/app.log

# 查看最近的错误
grep "ERROR" logs/app.log | tail -20

# 清空日志（不推荐，建议使用cleanup命令）
# > logs/app.log
```

### 数据备份
```bash
# 备份数据库
cp data/pushed_items.db data/pushed_items.db.backup

# 完整备份
tar -czf feedpilot-backup-$(date +%Y%m%d).tar.gz \
  /opt/feedpilot/data \
  /opt/feedpilot/logs \
  /opt/feedpilot/config \
  /opt/feedpilot/.env

# 下载备份到本地
# scp user@server:/opt/feedpilot/feedpilot-backup-*.tar.gz ./
```

## 🔥 故障排查

### 问题1: Docker Hub速率限制
```
Error: 429 Too Many Requests
```

**解决方案**:
```bash
# 方案1: 登录Docker Hub（推荐）
docker login

# 方案2: 先拉取基础镜像
docker pull python:3.11-slim

# 方案3: 使用重试脚本
bash scripts/build-retry.sh
```

详细说明: [docs/DOCKER-RATELIMIT.md](docs/DOCKER-RATELIMIT.md)

### 问题2: 容器无法启动
```bash
# 查看详细日志
docker-compose logs

# 检查环境变量
docker-compose exec feedpilot env | grep AI_

# 检查配置文件
docker-compose exec feedpilot cat config/app.yaml
```

### 问题2: 推送失败
```bash
# 测试推送
docker-compose exec feedpilot python main.py --test

# 检查PushPlus配置
docker-compose exec feedpilot cat config/app.yaml | grep -A 5 pushplus
```

### 问题3: AI分析失败
```bash
# 查看AI相关日志
docker-compose logs | grep -E "(阶段|AI|JSON)"

# 测试API连接
docker-compose exec feedpilot python -c "
from openai import OpenAI
import os
client = OpenAI(
    api_key=os.getenv('AI_API_KEY'),
    base_url=os.getenv('AI_API_BASE')
)
print('API连接成功')
"
```

### 问题4: 权限问题
```bash
# 修复目录权限
sudo chown -R $USER:$USER /opt/feedpilot
chmod -R 755 /opt/feedpilot
```

## 🔄 更新部署

```bash
cd /opt/feedpilot

# 停止服务
docker-compose down

# 更新代码（如果使用git）
git pull

# 重新构建镜像
docker-compose build --no-cache

# 启动服务
docker-compose up -d

# 验证
docker-compose logs -f
```

## 🛡️ 安全建议

### 1. 防火墙配置（可选）
```bash
# 如果服务器有UFW防火墙
sudo ufw status

# 通常Docker服务不需要开放额外端口
# 因为它只是定时推送，不对外提供服务
```

### 2. 定期更新
```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 更新Docker镜像基础层
docker-compose pull
docker-compose up -d
```

### 3. 日志轮转
```bash
# Docker已自动配置日志轮转（在docker-compose.yml中）
# 查看配置：
docker inspect feedpilot --format='{{.HostConfig.LogConfig}}'
```

## 📈 性能优化

### 资源限制
已在 `docker-compose.yml` 中配置：
- CPU限制: 1核
- 内存限制: 512MB

如需调整，编辑 `docker-compose.yml`:
```yaml
deploy:
  resources:
    limits:
      cpus: '2.0'      # 增加到2核
      memory: 1024M    # 增加到1GB
```

### 定时任务优化
编辑 `config/app.yaml`:
```yaml
scheduler:
  daily_time: "07:30"  # 调整推送时间
```

或通过环境变量：
```bash
# 编辑.env
DAILY_PUSH_TIME=08:00

# 重启服务
docker-compose restart
```

## 🔐 开机自启动

Docker容器默认已配置 `restart: unless-stopped`，系统重启后会自动启动。

验证：
```bash
sudo reboot  # 重启服务器

# 重启后检查
docker ps | grep feedpilot
```

## 📞 获取帮助

- 详细文档: `DOCKER.md`
- 部署检查清单: `DOCKER-CHECKLIST.md`
- 开发指南: `CLAUDE.md`
- 项目README: `README.md`

## ✅ 部署完成检查清单

- [ ] Docker和Docker Compose已安装
- [ ] 项目代码已上传到 `/opt/feedpilot`
- [ ] `.env` 文件已配置API密钥
- [ ] `config/app.yaml` 已配置RSS URL和PushPlus Token
- [ ] 容器成功启动（`docker-compose ps` 显示Up）
- [ ] 日志正常（无ERROR）
- [ ] 推送测试成功（`--test` 命令通过）
- [ ] 完整流程测试成功（`--once` 命令通过）

恭喜！部署完成！🎉
