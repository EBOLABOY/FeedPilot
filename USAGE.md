# RSS推送服务使用指南

## 🎯 快速开始

### 1. 核心机制

**重要:** 本服务**不按日期过滤**RSS内容,而是:
- ✅ 推送所有RSS源中的内容
- ✅ 数据库自动记录已推送内容
- ✅ 每次只推送未推送的新内容
- ✅ 永不重复推送

### 2. 已完成的功能

✅ **PushPlus群组推送**
- Token: `f3da240f668a4fa7ac8db1f9eaa16d39`
- 群组编号: `66`

✅ **完整功能**
- RSS抓取和解析
- SQLite数据库去重
- PushPlus推送
- 定时调度
- 日志系统

✅ **测试通过**
- ✓ RSS抓取: 50条目
- ✓ 去重推送: 每次5条
- ✓ 数据库防重复: 正常
- ✓ PushPlus推送: 成功

---

## 📝 当前配置

```yaml
# RSS订阅源
URL: http://47.79.39.147:8001/feed/all.rss
抓取间隔: 5分钟

# PushPlus推送
Token: f3da240f668a4fa7ac8db1f9eaa16d39
群组: 66
消息格式: HTML
单次最多推送: 5条

# 时区设置
默认时区: Asia/Shanghai (北京时间 UTC+8)
```

---

## 🚀 运行方式

### 方式1: 启动定时服务(推荐)
```bash
python main.py
```
程序将每5分钟自动抓取一次RSS,有新内容时推送到PushPlus群组66。

### 方式2: 手动执行一次
```bash
python main.py --once
```
立即执行一次抓取和推送,然后退出。

### 方式3: 测试连接
```bash
python main.py --test
```
测试PushPlus是否配置正确,会发送一条测试消息到群组。

### 方式4: 查看统计
```bash
python main.py --stats
```
查看历史推送统计信息。

---

## 📊 推送效果

### HTML格式推送示例:
```
📰 今日RSS推送 (5条)
━━━━━━━━━━━━━━━━━━━━━━

1. 新闻标题1
   📝 这是新闻摘要...
   🔗 查看详情
   📅 2025-10-11 12:00:00

2. 新闻标题2
   📝 这是新闻摘要...
   🔗 查看详情
   🖼️ [配图]
   📅 2025-10-11 11:30:00

...
```

推送消息包含:
- ✅ 标题和链接
- ✅ 文章摘要
- ✅ 配图(如果有)
- ✅ 发布时间
- ✅ 美化的HTML样式

---

## ⚙️ 配置调整

### 修改推送频率
编辑 `config/app.yaml`:
```yaml
rss:
  fetch_interval: 10  # 改为每10分钟
```

### 修改推送数量
```yaml
pushplus:
  message_template:
    max_items: 10  # 改为每次最多推送10条
```

### 修改消息格式
```yaml
pushplus:
  message_template:
    template: "markdown"  # 可选: html, markdown, txt
    include_description: false  # 不包含摘要
    include_image: false  # 不包含图片
```

### 启用推送时间窗口
```yaml
push:
  time_window:
    start: "09:00"
    end: "18:00"
    enabled: true  # 只在9:00-18:00推送
```

---

## 🐛 故障排查

### 问题1: 没有收到推送
**检查步骤:**
1. 运行 `python main.py --test` 测试连接
2. 查看日志文件 `logs/app.log`
3. 确认RSS源有今日新内容
4. 确认PushPlus的Token和群组编号正确

### 问题2: 显示"所有内容均已推送"
**原因:** RSS源中的所有内容都已经推送过了

**说明:**
- ✅ v1.1.0版本不再按日期过滤
- ✅ 只推送未推送的内容
- ✅ 数据库自动记录防重复
- 等待RSS源更新新内容即可自动推送

### 问题3: 推送重复
**不会发生:** 程序会自动记录已推送内容到SQLite数据库,避免重复推送。

### 问题4: 中文乱码
**已解决:** 日志系统使用UTF-8编码,配置文件也是UTF-8。

---

## 📁 项目文件说明

```
E:\RSS推送/
├── config/
│   └── app.yaml              # 配置文件(已配置好PushPlus)
├── data/
│   └── pushed_items.db       # SQLite数据库(自动创建)
├── logs/
│   └── app.log               # 日志文件(自动创建)
├── src/                      # 源代码
├── main.py                   # 主程序入口
├── requirements.txt          # Python依赖
├── README.md                 # 项目说明
└── USAGE.md                  # 本文件
```

---

## 🔄 后台运行

### Windows
使用 [NSSM](https://nssm.cc/):
```cmd
nssm install RSSPush "python" "E:\RSS推送\main.py"
nssm start RSSPush
```

### Linux
创建systemd服务:
```bash
sudo nano /etc/systemd/system/rss-push.service
```

内容:
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
```

---

## 📞 支持

如遇问题,请检查:
1. `logs/app.log` 日志文件
2. PushPlus官网是否正常
3. RSS源是否可访问

祝使用愉快! 🎉
