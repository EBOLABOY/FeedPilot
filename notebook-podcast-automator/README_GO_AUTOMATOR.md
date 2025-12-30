# NotebookLM Podcast Automator (Go Version)

This project provides a robust, purely Go-based solution to automate the extraction of WeChat articles and generation of NotebookLM podcasts.

## Key Features
1.  **WeChat Extraction**: Uses a specific `User-Agent` to bypass anti-scraping, implemented with standard `net/http` and `goquery`. No headless browser required for fetching.
2.  **Smart Cleaning**: Custom HTML cleaner (ported from FeedPilot) to extract relevant article content.
3.  **NotebookLM Integration**: Full API client implementation including specific fixes for Audio Generation (DirectRPC fallback).
4.  **Network Handling**: Optimized network configuration (Bypass proxy for WeChat, Use proxy for Google).

## Prerequisites
1.  **Authentication**: You must have `NLM_AUTH_TOKEN` and `NLM_COOKIES` in your `.env` file (migrated from `~/.nlm/env` or manually obtained).
2.  **Proxy**: Ensure your local proxy is running at `127.0.0.1:10809` (or update code).

## Usage

### Run the E2E Demo
The `demo_e2e_full.go` script demonstrates the entire flow:
1.  Fetches a WeChat article.
2.  Cleans the content.
3.  Creates a new NotebookLM project.
4.  Uploads the text source.
5.  Triggers Audio Overview generation.
6.  Polls for completion and downloads the audio.

```powershell
cd "e:\notebookllm 逆向\notebook-podcast-automator"
go run demo_e2e_full.go
```

## Technical Details
- **WeChat Fetching**: WeChat checks User-Agent strictly. We use a Chrome/Windows User-Agent to ensure access. Proxy is explicitly disabled for WeChat requests to avoid blocking/latency.
- **Audio Generation**: We use `client.SetUseDirectRPC(true)` to bypass the gRPC "Unavailable" error.
- **Language Customization**: To generate Chinese podcasts, the "Instruction" prompt must be in Chinese. The demo script has been updated with a Chinese prompt: `"请生成一段深入的中文播客对话..."`.
