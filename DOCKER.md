# Docker 部署指南

## 📦 快速开始

### 1. 准备配置文件

复制环境变量模板：
```bash
cp .env.docker.example .env
```

编辑 `.env` 文件，填入你的配置：
```bash
# 必填：AI API密钥
AI_API_KEY=sk-your-actual-api-key

# 可选：其他配置保持默认即可
```

### 2. 启动服务

**使用 Docker Compose（推荐）**：
```bash
# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

**或使用 Docker 命令**：
```bash
# 构建镜像
docker build -t feedpilot:latest .

# 运行容器
docker run -d \
  --name feedpilot \
  --restart unless-stopped \
  -e AI_API_KEY=your-api-key \
  -e AI_API_BASE=http://154.19.184.12:3000/v1 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  -v $(pwd)/config:/app/config \
  feedpilot:latest
```

## 📁 目录结构

```
FeedPilot/
├── Dockerfile              # Docker镜像构建文件
├── docker-compose.yml      # Docker Compose配置
├── .dockerignore          # Docker构建忽略文件
├── .env.docker.example    # 环境变量模板
├── .env                   # 实际环境变量(需创建)
├── data/                  # 数据持久化(挂载卷)
│   └── pushed_items.db    # SQLite数据库
├── logs/                  # 日志持久化(挂载卷)
│   └── app.log           # 应用日志
└── config/                # 配置文件(挂载卷)
    └── app.yaml          # 应用配置
```

## ⚙️ 环境变量说明

### 必填变量

| 变量名 | 说明 | 示例 |
|--------|------|------|
| `AI_API_KEY` | AI API密钥 | `sk-xxx...` |

### 可选变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `AI_API_BASE` | `http://154.19.184.12:3000/v1` | API地址 |
| `ENABLE_TWO_STAGE` | `true` | 是否启用两阶段筛选 |
| `ENABLE_FULL_TEXT` | `true` | 是否获取全文 |
| `STAGE1_MODEL` | `gemini-2.5-flash` | 阶段1模型 |
| `STAGE2_MODEL` | `gemini-2.5-pro` | 阶段2模型 |
| `STAGE1_SCORE_THRESHOLD` | `7` | 阶段1分数阈值 |
| `DAILY_PUSH_TIME` | `07:30` | 每日推送时间 |

## 🔧 常用命令

### 查看服务状态
```bash
docker-compose ps
```

### 查看实时日志
```bash
docker-compose logs -f
```

### 查看最近100行日志
```bash
docker-compose logs --tail=100
```

### 重启服务
```bash
docker-compose restart
```

### 停止并删除容器
```bash
docker-compose down
```

### 重新构建镜像
```bash
docker-compose build --no-cache
docker-compose up -d
```

### 进入容器调试
```bash
docker-compose exec feedpilot /bin/bash
```

### 手动执行一次推送
```bash
docker-compose exec feedpilot python main.py --once
```

### 查看推送统计
```bash
docker-compose exec feedpilot python main.py --stats
```

### 清理旧记录
```bash
docker-compose exec feedpilot python main.py --cleanup 30
```

## 📝 配置修改

### 方式1: 修改环境变量（推荐）

编辑 `.env` 文件，重启服务：
```bash
nano .env
docker-compose restart
```

### 方式2: 修改配置文件

编辑 `config/app.yaml`，重启服务：
```bash
nano config/app.yaml
docker-compose restart
```

### 方式3: 修改提示词

编辑 `阶段1提示词.md` 或 `阶段2提示词.md`，重启服务：
```bash
nano 阶段1提示词.md
docker-compose restart
```

## 🔍 故障排查

### 1. 容器无法启动

查看详细日志：
```bash
docker-compose logs feedpilot
```

常见原因：
- 环境变量未配置（检查 `.env` 文件）
- 端口冲突（检查是否有其他服务占用）
- 权限问题（检查 `data/` 和 `logs/` 目录权限）

### 2. 推送失败

检查配置：
```bash
# 进入容器
docker-compose exec feedpilot /bin/bash

# 查看配置
cat config/app.yaml

# 测试推送
python main.py --test
```

### 3. AI分析失败

检查API密钥和网络：
```bash
# 查看环境变量
docker-compose exec feedpilot env | grep AI_

# 测试API连接
docker-compose exec feedpilot python -c "
from openai import OpenAI
client = OpenAI(api_key='$AI_API_KEY', base_url='$AI_API_BASE')
print(client.models.list())
"
```

### 4. 数据库损坏

重建数据库：
```bash
docker-compose down
rm -f data/pushed_items.db
docker-compose up -d
```

## 📊 监控与维护

### 查看资源使用
```bash
docker stats feedpilot
```

### 查看日志大小
```bash
du -sh logs/
```

### 备份数据
```bash
# 备份数据库
cp data/pushed_items.db data/pushed_items.db.backup

# 或使用tar打包
tar -czf feedpilot-backup-$(date +%Y%m%d).tar.gz data/ logs/ config/
```

### 定期清理
```bash
# 清理30天前的记录
docker-compose exec feedpilot python main.py --cleanup 30

# 清理Docker日志
docker-compose down
rm -f $(docker inspect --format='{{.LogPath}}' feedpilot)
docker-compose up -d
```

## 🚀 生产环境建议

### 1. 使用外部配置管理

将配置文件独立管理：
```bash
# 在宿主机上准备配置
mkdir -p /opt/feedpilot/{data,logs,config}
cp config/app.yaml /opt/feedpilot/config/

# 修改docker-compose.yml中的volumes
volumes:
  - /opt/feedpilot/data:/app/data
  - /opt/feedpilot/logs:/app/logs
  - /opt/feedpilot/config:/app/config
```

### 2. 设置日志轮转

在 `docker-compose.yml` 中已配置：
```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
```

### 3. 配置健康检查

Docker会自动检查数据库可用性：
```bash
docker inspect feedpilot --format='{{.State.Health.Status}}'
```

### 4. 设置自动重启

已在 `docker-compose.yml` 中配置：
```yaml
restart: unless-stopped
```

### 5. 资源限制

根据实际需求调整 `docker-compose.yml` 中的资源限制：
```yaml
deploy:
  resources:
    limits:
      cpus: '1.0'
      memory: 512M
```

## 🐛 调试模式

临时启用DEBUG日志：
```bash
docker-compose down
docker-compose run --rm -e LOG_LEVEL=DEBUG feedpilot python main.py --once
```

## 📦 镜像管理

### 构建并推送到私有仓库
```bash
# 构建
docker build -t your-registry.com/feedpilot:v1.0 .

# 推送
docker push your-registry.com/feedpilot:v1.0

# 在其他机器上拉取
docker pull your-registry.com/feedpilot:v1.0
```

### 导出导入镜像
```bash
# 导出
docker save feedpilot:latest -o feedpilot.tar

# 导入
docker load -i feedpilot.tar
```

## 🔐 安全建议

1. **不要将 `.env` 文件提交到Git**
   ```bash
   echo ".env" >> .gitignore
   ```

2. **定期更新基础镜像**
   ```bash
   docker-compose build --pull
   ```

3. **使用只读挂载（可选）**
   ```yaml
   volumes:
     - ./config:/app/config:ro
     - ./阶段1提示词.md:/app/阶段1提示词.md:ro
   ```

4. **限制容器权限**
   ```yaml
   security_opt:
     - no-new-privileges:true
   ```

## 📚 更多资源

- [Docker官方文档](https://docs.docker.com/)
- [Docker Compose文档](https://docs.docker.com/compose/)
- [FeedPilot项目README](README.md)
- [CLAUDE.md - 开发指南](CLAUDE.md)
