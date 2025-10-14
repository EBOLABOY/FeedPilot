# RSS推送服务

一个自动化的RSS订阅源抓取和推送服务,支持将RSS新闻推送到PushPlus群组。

## 功能特性

- ✅ 自动抓取RSS订阅源
- ✅ **两阶段AI智能筛选** (快速打分→全文分析)
- ✅ 智能过滤未推送内容(数据库去重)
- ✅ 去重和排序处理
- ✅ 推送到PushPlus群组
- ✅ 支持HTML/Markdown/纯文本格式
- ✅ SQLite数据库防止重复推送
- ✅ 定时调度自动执行
- ✅ 推送时间窗口控制(可选)
- ✅ 完善的日志系统

**核心机制:** 不按日期过滤,推送所有RSS源中未推送的内容,数据库自动防止重复推送。

**AI增强:** 支持两阶段筛选 - 先用快速模型打分筛选,再获取全文深度分析,提高准确率同时降低成本。

## 项目结构

```
RSS推送/
├── config/
│   └── app.yaml              # 配置文件
├── data/                     # 数据库存储目录
├── logs/                     # 日志文件目录
├── src/
│   ├── ai/                  # AI内容增强
│   │   └── content_enhancer.py
│   ├── config/              # 配置管理
│   │   └── loader.py
│   ├── db/                  # 数据库存储
│   │   └── storage.py
│   ├── models/              # 数据模型
│   │   └── rss_item.py
│   ├── pushers/             # 推送器
│   │   ├── base.py
│   │   └── pushplus.py
│   ├── rss/                 # RSS处理
│   │   ├── fetcher.py
│   │   └── parser.py
│   └── utils/               # 工具函数
│       ├── content_fetcher.py  # 网页内容抓取
│       └── logger.py
├── .env                     # 环境变量(需自行创建)
├── 系统提示词.md            # AI分析系统提示词
├── main.py                  # 主程序入口
└── requirements.txt         # Python依赖

```

## 安装依赖

```bash
pip install -r requirements.txt
```

## 配置说明

### 1. 创建环境变量文件

复制 `.env.example` 为 `.env` 并配置:

```bash
# ========== 阶段1: 初筛模型配置 ==========
# 用于快速筛选RSS条目(仅基于标题+摘要)
STAGE1_API_BASE=http://your-api-base/v1
STAGE1_API_KEY=your-api-key
STAGE1_MODEL=gemini-1.5-flash  # 建议使用快速便宜的模型
STAGE1_SCORE_THRESHOLD=7  # 初筛分数阈值(7-10分的文章进入第二阶段)

# ========== 阶段2: 深度分析模型配置 ==========
# 用于深度分析全文内容
STAGE2_API_BASE=http://your-api-base/v1
STAGE2_API_KEY=your-api-key
STAGE2_MODEL=gemini-2.5-pro  # 建议使用强大的模型

# ========== 两阶段筛选配置 ==========
ENABLE_TWO_STAGE=true  # 是否启用两阶段筛选
ENABLE_FULL_TEXT=true  # 是否获取全文

# 定时推送时间 (HH:MM格式, 24小时制)
# 单个时间: DAILY_PUSH_TIME=07:30
# 多个时间: DAILY_PUSH_TIME=08:15,18:30 (用逗号分隔)
DAILY_PUSH_TIME=07:30
```

### 2. 编辑配置文件

编辑 `config/app.yaml` 文件:

```yaml
# RSS订阅源配置
rss:
  url: "http://your-rss-feed-url/feed.rss"

# PushPlus配置
pushplus:
  token: "your-pushplus-token"      # 你的PushPlus Token
  topic: "66"                        # 群组编号
  message_template:
    max_items: 20                    # 一次推送最多几条新闻
    include_description: true        # 是否包含描述
    include_image: false             # 是否包含图片
    template: "markdown"             # 消息格式: html, markdown, txt

# 内容增强配置(两阶段筛选)
content_enhancer:
  enabled: true                      # 是否启用AI内容增强
  provider: "openai"                 # openai 或 claude
```

### 3. 配置系统提示词(可选)

编辑 `系统提示词.md` 文件来自定义AI分析规则,详见 `内容增强使用指南.md`。

## 快速开始

### Python直接部署

```bash
# 快速修复并部署（自动安装依赖）
sudo bash scripts/quickfix-deploy.sh

# 或手动安装依赖
sudo apt install -y python3-venv python3-pip
bash scripts/deploy-python.sh

# 配置systemd服务（后台运行）
sudo bash scripts/setup-systemd.sh
```

详细说明请查看：
- [Ubuntu部署指南](UBUNTU-DEPLOY.md)

## 常用命令

### 仅执行一次
```bash
python main.py --once
```

### 测试推送器连接
```bash
python main.py --test
```

### 查看推送统计
```bash
python main.py --stats
```

### 清理旧记录
```bash
python main.py --cleanup 30  # 清理30天前的记录
```

### 使用自定义配置文件
```bash
python main.py --config /path/to/config.yaml
```

## 推送格式示例

### HTML格式 (默认)
推送消息将包含:
- 标题和链接
- 文章摘要
- 配图(如果有)
- 发布时间
- 美化的HTML样式

### Markdown格式
适合支持Markdown的推送渠道,格式清晰简洁。

### 纯文本格式
最简单的文本格式,兼容性最好。

## 配置选项

### RSS配置
- `url`: RSS订阅源地址
- `fetch_interval`: 抓取间隔(分钟)

### PushPlus配置
- `token`: PushPlus的Token
- `topic`: 群组编号
- `message_template.max_items`: 单次推送最大条目数
- `message_template.template`: 消息格式(html/markdown/txt)

### 时间窗口配置
```yaml
push:
  time_window:
    start: "09:00"
    end: "18:00"
    enabled: true  # 启用后仅在此时间段推送
```

### 日志配置
```yaml
logging:
  level: "INFO"           # DEBUG, INFO, WARNING, ERROR
  file: "logs/app.log"
  max_size: "10MB"
  backup_count: 5
```

### 数据库配置
```yaml
database:
  type: "sqlite"
  path: "data/pushed_items.db"
```

## 常见问题

### 1. 如何获取PushPlus Token?
访问 [PushPlus官网](http://www.pushplus.plus/) 注册并获取Token。

### 2. 推送失败怎么办?
- 检查Token和Topic是否正确
- 运行 `python main.py --test` 测试连接
- 查看 `logs/app.log` 日志文件

### 3. 如何避免重复推送?
程序会自动记录已推送的内容到SQLite数据库,避免重复推送。

### 4. 如何修改推送频率?
修改 `config/app.yaml` 中的 `rss.fetch_interval` 值(单位:分钟)。

### 5. 如何部署为后台服务?

**推荐方式: 使用systemd服务**

运行部署脚本自动配置:
```bash
sudo bash scripts/setup-systemd.sh
```

或手动创建 `/etc/systemd/system/feedpilot.service`:
```ini
[Unit]
Description=FeedPilot RSS Push Service
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/opt/feedpilot
ExecStart=/usr/bin/python3 /opt/feedpilot/main.py
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务:
```bash
sudo systemctl enable feedpilot
sudo systemctl start feedpilot
sudo systemctl status feedpilot
```

## 开发与扩展

### 添加新的推送器
1. 在 `src/pushers/` 创建新的推送器类
2. 继承 `BasePusher` 抽象类
3. 实现必需的方法
4. 在 `main.py` 中注册新推送器

### 自定义消息格式
修改 `src/pushers/pushplus.py` 中的 `_format_*_message` 方法。

## 许可证

MIT License

## 作者

RSS推送服务项目

## 更新日志

### v1.0.0 (2024-10-10)
- 初始版本发布
- 支持PushPlus推送
- 完整的RSS抓取和解析功能
- SQLite数据库支持
- 定时调度功能
