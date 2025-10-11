# RSS推送服务 - 最终配置说明

## ✅ 完成配置 (v1.2.0)

### 🎯 核心特性

**调度方式**: 每天早上 7:30 定时执行一次
**推送策略**: 推送所有未推送的RSS内容,数据库自动去重
**单次限制**: 每次最多推送20条(防止内容过长)
**分批推送**: 如果超过20条,会分多天推送完

---

## 📊 当前配置

```yaml
调度类型: daily (每天定时)
执行时间: 07:30 (北京时间)
RSS源: http://47.79.39.147:8001/feed/all.rss
推送方式: PushPlus
Token: f3da240f668a4fa7ac8db1f9eaa16d39
群组: 66
消息格式: HTML
单次推送: 最多20条
包含描述: 是
包含图片: 否 (减少内容长度)
```

---

## 🔄 工作流程

### 每天 7:30 自动执行:

```
1. 抓取RSS源(50条)
   ↓
2. 去重排序
   ↓
3. 查询数据库,筛选未推送的
   ↓
4. 限制为最多20条(防止超长)
   ↓
5. 推送到PushPlus群组66
   ↓
6. 标记已推送的20条到数据库
   ↓
7. 剩余30条等待明天推送
```

### 第二天 7:30:
```
1. 抓取RSS源
   ↓
2. 筛选未推送(还有30条旧的+可能的新内容)
   ↓
3. 推送20条
   ↓
4. 还剩10条+新内容等待后天
```

---

## 📅 示例场景

### 场景1: RSS源有50条未推送内容
- **第1天 7:30**: 推送前20条
- **第2天 7:30**: 推送第21-40条
- **第3天 7:30**: 推送第41-50条
- **第4天 7:30**: 显示"所有内容均已推送"

### 场景2: RSS源每天更新10条
- **每天 7:30**: 推送当天的10条新内容
- 不会积压,不会重复

### 场景3: RSS源突然更新100条
- **第1天**: 推送20条
- **第2天**: 推送20条
- **第3天**: 推送20条
- **第4天**: 推送20条
- **第5天**: 推送20条
- **第6天**: 显示"所有内容均已推送"

---

## 🚀 启动方式

### 1. Windows - 作为后台服务运行(推荐)

使用 NSSM 创建Windows服务:

```cmd
# 下载 NSSM: https://nssm.cc/download
# 以管理员身份运行:
nssm install RSSPush "python" "E:\RSS推送\main.py"
nssm start RSSPush

# 查看状态
nssm status RSSPush

# 停止服务
nssm stop RSSPush

# 卸载服务
nssm remove RSSPush
```

### 2. Linux - 使用systemd

创建 `/etc/systemd/system/rss-push.service`:

```ini
[Unit]
Description=RSS Push Service
After=network.target

[Service]
Type=simple
WorkingDirectory=/path/to/RSS推送
ExecStart=/usr/bin/python3 /path/to/RSS推送/main.py
Restart=always

[Install]
WantedBy=multi-user.target
```

启动:
```bash
sudo systemctl enable rss-push
sudo systemctl start rss-push
sudo systemctl status rss-push
```

### 3. 前台运行(调试用)

```bash
python main.py
```

程序会显示:
```
启动定时调度器,每天 07:30 执行一次
下次执行时间: 07:30
等待定时任务触发...
```

---

## 🔧 常用命令

### 手动执行一次(不等到7:30)
```bash
python main.py --once
```

### 测试PushPlus连接
```bash
python main.py --test
```

### 查看推送统计
```bash
python main.py --stats
```

### 清理30天前的记录
```bash
python main.py --cleanup 30
```

---

## ⚙️ 配置调整

### 修改执行时间

编辑 `config/app.yaml`:
```yaml
scheduler:
  daily_time: "08:00"  # 改为早上8点
```

### 修改单次推送数量

```yaml
pushplus:
  message_template:
    max_items: 30  # 改为30条(如果内容不会太长)
```

### 修改为按间隔执行(不推荐)

```yaml
scheduler:
  schedule_type: "interval"  # 改为按间隔

rss:
  fetch_interval: 60  # 每60分钟执行一次
```

---

## 📊 监控和维护

### 查看日志
```bash
# 查看最近的日志
tail -f logs/app.log

# 查看今天的推送记录
grep "推送成功" logs/app.log | grep "$(date +%Y-%m-%d)"
```

### 数据库位置
```
data/pushed_items.db
```

### 清空数据库(重新推送所有内容)
```bash
rm data/pushed_items.db
```

---

## ✅ 优势

1. **不会遗漏**: 推送所有RSS内容,不按日期过滤
2. **不会重复**: 数据库自动记录已推送,永不重复
3. **稳定可靠**: 每天固定时间执行,不会积压
4. **自动分批**: 内容太多时自动分多天推送
5. **资源友好**: 每天只执行一次,不占用资源

---

## 🎉 使用建议

### 日常使用
- 让程序作为后台服务运行
- 每天早上7:30自动推送
- 无需人工干预

### 调试阶段
- 使用 `--once` 手动测试
- 使用 `--stats` 查看统计
- 查看日志了解详情

### 维护建议
- 定期查看日志确认正常运行
- 每月清理一次旧数据库记录
- RSS源更新时测试一下

---

## 📞 快速参考

```bash
# 启动服务
python main.py

# 立即推送一次
python main.py --once

# 查看统计
python main.py --stats

# 测试连接
python main.py --test

# 查看日志
tail -f logs/app.log

# 清空数据库
rm data/pushed_items.db
```

---

**完成日期**: 2025-10-11
**版本**: v1.2.0
**状态**: ✅ 生产就绪

祝使用愉快! 🎊
