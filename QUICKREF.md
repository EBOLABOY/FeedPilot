# 🚀 FeedPilot 快速命令参考

## 一键部署

```bash
# 在Ubuntu服务器上执行
curl -fsSL https://get.docker.com | sh
docker-compose up -d
```

## 常用命令

| 操作 | 命令 |
|------|------|
| 启动服务 | `docker-compose up -d` |
| 停止服务 | `docker-compose down` |
| 重启服务 | `docker-compose restart` |
| 查看状态 | `docker-compose ps` |
| 查看日志 | `docker-compose logs -f` |
| 测试推送 | `docker-compose exec feedpilot python main.py --test` |
| 手动执行 | `docker-compose exec feedpilot python main.py --once` |
| 查看统计 | `docker-compose exec feedpilot python main.py --stats` |
| 清理数据 | `docker-compose exec feedpilot python main.py --cleanup 30` |

## 配置修改

```bash
# 修改环境变量
nano .env && docker-compose restart

# 修改配置文件
nano config/app.yaml && docker-compose restart

# 修改提示词
nano 阶段1提示词.md && docker-compose restart
```

## 故障排查

```bash
# 查看完整日志
docker-compose logs

# 查看错误日志
docker-compose logs | grep ERROR

# 检查容器状态
docker inspect feedpilot

# 重建容器
docker-compose down && docker-compose up -d --build
```

## 更新部署

```bash
git pull
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

## 备份恢复

```bash
# 备份
tar -czf backup-$(date +%Y%m%d).tar.gz data/ logs/ config/ .env

# 恢复
tar -xzf backup-*.tar.gz
```

---

详细文档：
- **Ubuntu部署**: [UBUNTU-DEPLOY.md](UBUNTU-DEPLOY.md)
- **Docker文档**: [DOCKER.md](DOCKER.md)
- **部署清单**: [DOCKER-CHECKLIST.md](DOCKER-CHECKLIST.md)
