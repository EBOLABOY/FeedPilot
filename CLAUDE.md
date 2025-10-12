# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

FeedPilot (RSS推送服务) is an automated RSS feed fetching and push notification service that:
- Fetches RSS feeds on a scheduled basis
- Filters content using AI scoring (OpenAI/Claude compatible APIs or rule-based)
- Prevents duplicate pushes using SQLite database
- Pushes filtered content to PushPlus group messaging service
- Supports HTML, Markdown, and text formats

## Key Commands

### Development & Testing
```bash
# Install dependencies
pip install -r requirements.txt

# Run once (single execution, no scheduling)
python main.py --once

# Test pusher connection
python main.py --test

# View push statistics
python main.py --stats

# Clean up old records (e.g., 30 days)
python main.py --cleanup 30

# Use custom config file
python main.py --config /path/to/config.yaml

# Debug RSS feed parsing
python debug_rss.py
```

### Production
```bash
# Start scheduled service (runs continuously)
python main.py
```

## Architecture

### Core Flow (main.py:107-200)
1. **Fetch RSS** - RSSFetcher retrieves feed from configured URL
2. **Parse Items** - RSSParser converts feed entries to RSSItem objects
3. **Deduplicate** - Remove duplicates within current batch
4. **Filter Unpushed** - PushStorage checks SQLite DB to exclude already-pushed items
5. **Content Enhancement** - (Optional) ContentEnhancer performs deep analysis with custom system prompts
6. **Push** - Send enhanced/original content via configured pusher(s)
7. **Mark as Pushed** - Record pushed items in database

### Component Structure

**Configuration System** (`src/config/loader.py`)
- `ConfigLoader` - Loads and validates YAML config
- Uses singleton pattern via `get_config()` function
- Supports dot-notation access (e.g., `get('rss.url')`)

**RSS Processing**
- `RSSFetcher` (`src/rss/fetcher.py`) - Fetches RSS feeds via HTTP
- `RSSParser` (`src/rss/parser.py`) - Parses, deduplicates, and sorts items
- `RSSItem` (`src/models/rss_item.py`) - Data model for RSS entries
  - Uses `guid` field as unique identifier, falls back to `link` if no GUID

**Storage** (`src/db/storage.py`)
- `PushStorage` - SQLite database manager
- Core method: `filter_unpushed_items()` - Prevents duplicate pushes
- Tracks push history with timestamps, pusher name, and success status

**Content Enhancement** (`src/ai/content_enhancer.py`)
- `ContentEnhancer` - Deep analysis using custom system prompts (e.g., for exam preparation)
- Loads system prompt from `系统提示词.md` in project root
- Transforms RSS items into structured Markdown analysis (tables, categories, ratings)
- Automatically includes clickable links in Markdown format: `[Title](URL)`
- Supports OpenAI and Claude API providers
- API credentials read from `.env` file: `AI_API_KEY`, `AI_API_BASE`, `AI_MODEL`
- Output is pushed directly as formatted Markdown via `push_custom_message()`

**Pushers** (`src/pushers/`)
- `BasePusher` - Abstract base class defining pusher interface
- `PushPlusPusher` - PushPlus API implementation
- Key methods:
  - `initialize()`, `push_items()`, `validate_config()`, `test_connection()`
  - `push_custom_message(title, content, template)` - For enhanced content (Markdown/HTML/text)

### Configuration (`config/app.yaml`)

**Critical Settings:**
- `rss.url` - RSS feed URL to monitor
- `pushplus.token` - PushPlus API token (required)
- `pushplus.topic` - PushPlus group ID (required)
- `content_enhancer.enabled` - Toggle content enhancement with custom prompts (true/false)
- `content_enhancer.provider` - API provider ("openai" or "claude")
- `scheduler.schedule_type` - "daily" (runs at fixed time) or "interval" (runs every N minutes)

**Scheduler Behavior:**
- `schedule_type: "daily"` with `daily_time: "07:30"` - Runs once per day at 7:30 AM
- `schedule_type: "interval"` with `rss.fetch_interval: 5` - Runs every 5 minutes
- Time window (`push.time_window`) can restrict pushes to specific hours

**Deduplication Strategy:**
The service pushes ALL unpushed items from the RSS feed, not just "today's" items. The database (`PushStorage`) automatically prevents duplicates by tracking GUIDs.

## Adding New Pushers

1. Create new file in `src/pushers/` (e.g., `telegram.py`)
2. Inherit from `BasePusher` abstract class
3. Implement required methods:
   - `initialize()` - Setup and connection test
   - `push_items(items: List[RSSItem])` - Send items via API
   - `validate_config()` - Verify configuration is valid
   - `test_connection()` - Test API connectivity
4. Register in `main.py:64-88` in `_init_pushers()` method
5. Add pusher config section to `config/app.yaml`
6. Add pusher name to `push.enabled_pushers` list

## Environment Variables

Create `.env` file for sensitive credentials (see `.env.example`):
- `AI_API_KEY` - API key for AI scoring service
- `AI_API_BASE` - API base URL (optional, for custom endpoints)
- `AI_MODEL` - Model name (e.g., "gpt-3.5-turbo")

## Content Enhancement

The project uses a **Content Enhancer** system for intelligent RSS analysis:

**Features:**
- Deep analysis using domain-specific system prompts (e.g., exam preparation, specialized filtering)
- Structured output with tables, categories, and ratings in Markdown
- Clickable links in the format `[Title](URL)` for direct navigation
- Supports OpenAI and Claude API providers

**Setup:**
1. Create system prompt in `系统提示词.md` at project root
2. Configure API credentials in `.env` file
3. Enable in `config/app.yaml`: `content_enhancer.enabled: true`

**Output Example:**
```markdown
## 核心必读 (★★★★★)
| 文章标题 | 权重 | 推荐理由 |
|---------|------|---------|
| [教育强国建设规划](http://...) | ★★★★★ | 紧扣国家顶层设计... |
```

## Important Notes

- **Database Location:** `data/pushed_items.db` (created automatically)
- **Logs:** Written to `logs/app.log` with rotation (configurable in logging section)
- **Date Parsing:** Uses `dateutil` library if available for better date parsing (`pip install python-dateutil`)
- **Image Extraction:** `RSSItem.extract_first_image()` finds first `<img>` tag in description
- **Timezone Handling:** `RSSParser` accepts `timezone_offset` parameter (default: +8 for Asia/Shanghai)
- **System Prompt Location:** `系统提示词.md` in project root for ContentEnhancer
