# ⚡ Docker Hub 速率限制 - 快速解决指南

## 🚨 错误信息
```
429 Too Many Requests - You have reached your unauthenticated pull rate limit
```

## ✅ 三种快速解决方案

### 方案1: 登录Docker Hub（最推荐）⭐

```bash
# 直接登录
docker login

# 输入你的Docker Hub用户名和密码
# 没有账户? 免费注册: https://hub.docker.com/signup

# 然后重新构建
docker-compose build
```

### 方案2: 先拉取基础镜像

```bash
# 手动拉取Python镜像
docker pull python:3.11-slim

# 等待拉取完成后，再构建
docker-compose build
```

### 方案3: 等待并重试

```bash
# 使用自动重试脚本（等待60秒后重试）
bash scripts/build-retry.sh

# 或者手动等待6小时后重试（速率限制会重置）
```

## 📊 限额说明

| 用户类型 | 拉取限额 | 时间窗口 |
|---------|---------|---------|
| 匿名用户 | 100次 | 6小时 |
| 免费登录用户 | 200次 | 6小时 |
| 付费用户 | 无限制 | - |

## 🔍 检查当前状态

```bash
# 检查是否已登录
docker info | grep Username

# 查看本地Python镜像
docker images | grep python

# 如果看到python:3.11-slim，说明本地已有，不需要下载
```

## 💡 为什么会遇到这个问题？

Docker构建时需要基础镜像：
```dockerfile
FROM python:3.11-slim  ← 这个镜像需要从Docker Hub下载
```

如果本地没有这个镜像，Docker会自动下载。但Docker Hub限制了下载次数。

## 🎯 推荐做法

**部署前先登录Docker Hub：**

```bash
# 1. 注册免费账户（如果没有）
# https://hub.docker.com/signup

# 2. 登录
docker login

# 3. 正常部署
bash deploy.sh
```

## 📚 更多信息

- 详细说明: [docs/DOCKER-RATELIMIT.md](DOCKER-RATELIMIT.md)
- Docker原理: [docs/WHY-DOCKER-PULL.md](WHY-DOCKER-PULL.md)
- 部署指南: [UBUNTU-DEPLOY.md](../UBUNTU-DEPLOY.md)
