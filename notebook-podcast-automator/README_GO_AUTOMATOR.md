# NotebookLM Podcast Automator (Go Version)

This project provides a robust, purely Go-based solution to automate the extraction of WeChat articles and generation of NotebookLM podcasts.

## Key Features
1.  **WeChat Extraction**: Uses a specific `User-Agent` to bypass anti-scraping, implemented with standard `net/http` and `goquery`. No headless browser required for fetching.
2.  **Smart Cleaning**: Custom HTML cleaner to extract relevant article content.
3.  **NotebookLM Integration**: Full API client implementation including specific fixes for Audio Generation (DirectRPC fallback).
4.  **Network Handling**: Optimized network configuration (Bypass proxy for WeChat, Use proxy for Google).

## Prerequisites
1.  **Authentication**: You must have `NLM_AUTH_TOKEN` and `NLM_COOKIES` in your `.env` file (migrated from `~/.nlm/env` or manually obtained).
2.  **Proxy (Optional)**: Set `NLM_PROXY_URL` or `HTTP_PROXY`/`HTTPS_PROXY`. If none is set, the server will auto-detect a local proxy at `127.0.0.1:10809`.

## Usage

### HTTP Service (Single Entry)
This project is designed to be driven by HTTP only.

- `GET /status`: health + last run snapshot
- `POST /run`: trigger end-to-end workflow (default input uses `NPA_INPUT_URL`)
- `POST /rss/prune`: 裁剪远端 `feed.xml`（仅在已配置 R2 时生效），用于“只保留最新 N 期节目”
- `noop=true`: 表示本次运行“无新文章/全部被过滤”，属于成功但不产出播客（不会返回 500）

### Daily Scheduler (Built-in)
Set these env vars to let the server auto-run once per day:

- `NPA_SCHEDULE_AT=06:00`
- `NPA_SCHEDULE_TZ=Asia/Shanghai` (optional; falls back to `NPA_TZ` or system local time)

### Dedup State (Processed/Skipped)
The server maintains a local state file to skip articles that were already handled (including those filtered out by rules/LLM):

- `NPA_STATE_PATH=data/state.json` (default)
- `NPA_STATE_MAX_ENTRIES=100` keeps the state bounded (max 100; oldest entries pruned)
- `NPA_STATE_DISABLED=true` to disable dedup
- `NPA_MAX_ENTRIES=50` controls how many *new* entries to process per run (it scans the feed until it finds enough new ones)

### RSS Retention (Optional)
To keep the RSS feed bounded (e.g. testing), set:

- `PODCAST_MAX_ITEMS=1` keeps only the latest episode in `feed.xml` after each successful update.

### Apple Podcasts (RSS / Metadata)
Apple Podcasts Connect pulls show-level metadata from the RSS `<channel>` (e.g. artist/author, owner, cover).
Set these in `.env`, then trigger a rewrite (e.g. `POST /rss/prune`) to update `feed.xml` on R2:

- `PODCAST_AUTHOR` (required by Apple as "Artist")
- `PODCAST_OWNER_NAME`, `PODCAST_OWNER_EMAIL` (required for verification)
- `PODCAST_COVER_URL` (optional; defaults to `<R2_PUBLIC_URL>/cover.jpg`)
  - Cover must be JPG/PNG, **3000x3000**, RGB, and the host must support **HTTP HEAD** and **Range** requests.
- `PODCAST_ITUNES_CATEGORY` (recommended; defaults to `Education`)
- `PODCAST_ITUNES_EXPLICIT` (recommended; defaults to `no`)
- `PODCAST_ITUNES_TYPE` (optional; defaults to `episodic`)
- `PODCAST_ITUNES_SUMMARY` (optional; defaults to `PODCAST_DESCRIPTION`)

```powershell
cd "D:\FeedPilot-1\notebook-podcast-automator"
go run .

Invoke-RestMethod http://localhost:8080/status

# 基础运行（抓取 -> 生成 -> 下载；若配置了 R2 则自动上传+更新 RSS）
$body = @{
  input_url = 'http://192.168.100.3:10082/atom'
  max_entries = 3
} | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/run' -ContentType 'application/json' -Body $body

# 仅保留 RSS 最新 1 期节目（会修改 R2 上的 feed.xml）
$body = @{ max_items = 1 } | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/rss/prune' -ContentType 'application/json' -Body $body

# 规则过滤示例（过滤报名/考试类低价值内容）
$body = @{
  input_url = 'http://192.168.100.3:10082/atom'
  max_entries = 10
  filter_mode = 'rules'
  filter_block_keywords = @('报名','考试','招聘','公示')
  filter_min_content_chars = 800
} | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/run' -ContentType 'application/json' -Body $body

# LLM 过滤示例（OpenAI 兼容；API Key 仅从环境变量读取）
# 在 .env 或当前终端设置：
#   NPA_FILTER_LLM_BASE_URL=https://api.openai.com/v1
#   NPA_FILTER_LLM_MODEL=gpt-4o-mini
#   NPA_FILTER_LLM_API_KEY=xxxxx
#   NPA_FILTER_LLM_TIMEOUT_SECONDS=300
#   NPA_FILTER_LLM_RETRIES=2
$body = @{
  input_url = 'http://192.168.100.3:10082/atom'
  max_entries = 10
  filter_mode = 'llm'
} | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/run' -ContentType 'application/json' -Body $body

# 推荐：标题初筛（招教/考试等）-> LLM 深度筛选 -> 仅上传保留文章到 NotebookLM
$body = @{
  input_url = 'http://192.168.100.3:10082/atom'
  max_entries = 30
  filter_mode = 'hybrid'
  filter_block_keywords = @('招教','考试','报名','招聘','公示')
  # 不截断（默认就是 0，可省略）
  filter_llm_max_chars = 0
} | ConvertTo-Json -Depth 6
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/run' -ContentType 'application/json' -Body $body
```

## CI: Build & Push to Docker Hub (GitHub Actions)
This repo includes a workflow that builds the Docker image and pushes it to Docker Hub on every push to the `go-podcast-automator` branch (and on tags `v*`).

### Required GitHub Secrets
Add these in GitHub: `Settings -> Secrets and variables -> Actions -> Secrets`:

- `DOCKERHUB_USERNAME`: your Docker Hub username (or org name)
- `DOCKERHUB_TOKEN`: a Docker Hub access token (recommended) or password

### Optional GitHub Variable
Add this in GitHub: `Settings -> Secrets and variables -> Actions -> Variables`:

- `DOCKERHUB_IMAGE`: override image name (e.g. `yourname/notebook-podcast-automator`)

If not set, the workflow uses `${DOCKERHUB_USERNAME}/notebook-podcast-automator`.

### Image Tags
- `latest` (from `go-podcast-automator` branch)
- `sha-<short>`
- `v*` tags (if you push git tags like `v1.0.0`)

## Technical Details
- **WeChat Fetching**: WeChat checks User-Agent strictly. We use a Chrome/Windows User-Agent to ensure access. Proxy is explicitly disabled for WeChat requests to avoid blocking/latency.
- **Audio Generation**: The client will fallback to DirectRPC if the orchestration service returns `Unavailable`.
- **Language Customization**: To generate Chinese podcasts, the "Instruction" prompt must be in Chinese. The demo script has been updated with a Chinese prompt: `"请生成一段深入的中文播客对话..."`.
