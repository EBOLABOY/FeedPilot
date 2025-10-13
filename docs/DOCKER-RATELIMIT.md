# Docker Hub 速率限制解决方案

## 问题说明
```
429 Too Many Requests - You have reached your unauthenticated pull rate limit
```

Docker Hub对匿名用户有拉取限制：
- **匿名用户**: 100次/6小时
- **免费登录用户**: 200次/6小时
- **付费用户**: 无限制

## 解决方案

### 🚀 方案1: 登录Docker Hub（推荐）

#### 1.1 注册账户（如果没有）
访问: https://hub.docker.com/signup

#### 1.2 使用登录脚本
```bash
bash scripts/docker-login.sh
```

#### 1.3 手动登录
```bash
docker login
# 输入用户名和密码
```

#### 1.4 重新构建
```bash
docker-compose build
```

### ⏳ 方案2: 等待重试

#### 2.1 使用自动重试脚本
```bash
bash scripts/build-retry.sh
```

#### 2.2 手动等待
速率限制会在6小时后重置，可以等待后重试。

### 📥 方案3: 预先拉取镜像

```bash
# 先拉取基础镜像
docker pull python:3.11-slim

# 然后再构建
docker-compose build
```

### 🔄 方案4: 使用缓存的镜像

如果之前已经拉取过Python镜像：
```bash
# 查看已有镜像
docker images | grep python

# 直接构建（会使用缓存）
docker-compose build --no-cache
```

## 快速诊断

### 检查当前状态
```bash
# 检查是否已登录
docker info | grep Username

# 查看本地镜像
docker images

# 测试拉取
docker pull hello-world
```

### 查看剩余配额
```bash
# 使用curl检查（需要登录凭证）
TOKEN=$(curl -s "https://auth.docker.io/token?service=registry.docker.io&scope=repository:ratelimitpreview/test:pull" | jq -r .token)
curl -s -H "Authorization: Bearer $TOKEN" https://registry-1.docker.io/v2/ratelimitpreview/test/manifests/latest -I | grep RateLimit
```

## 最佳实践

### 1. 登录Docker Hub
```bash
# 登录后限额翻倍
docker login
```

### 2. 使用本地缓存
```bash
# Docker会自动缓存已拉取的层
# 不要频繁使用 --no-cache
docker-compose build
```

### 3. 减少构建次数
```bash
# 开发时使用volume挂载，避免重复构建
# 在docker-compose.yml中已配置
volumes:
  - ./src:/app/src
```

### 4. 使用多阶段构建（已实现）
Dockerfile已优化，最小化层数。

## 长期解决方案

### 选项1: 使用付费Docker Hub账户
- **Pro**: $5/月，无限拉取
- **Team**: $7/月/用户，无限拉取

### 选项2: 使用私有镜像仓库
- 阿里云容器镜像服务
- 腾讯云容器镜像服务
- AWS ECR
- Google Container Registry

### 选项3: 自建镜像仓库
```bash
docker run -d -p 5000:5000 --name registry registry:2
```

## 当前建议

**对于日本服务器，最快的解决方案：**

1. **立即执行**:
   ```bash
   docker login
   docker-compose build
   ```

2. **如果还是失败**，先手动拉取：
   ```bash
   docker pull python:3.11-slim
   docker-compose build
   ```

3. **长期使用**，建议创建免费Docker Hub账户并保持登录状态。

## 预防措施

### 在deploy.sh中添加检查
已在脚本中添加Docker检查，建议登录后再部署。

### CI/CD配置
如果使用CI/CD，记得在pipeline中配置Docker Hub凭证。

---

**注意**: 速率限制是按IP地址计算的，如果服务器被多人共用，可能更容易触发限制。
