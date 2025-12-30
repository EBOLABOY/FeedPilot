package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/cleaner"
	"notebook-podcast-automator/internal/uploader"
)

// PodcastEpisode 贯穿全流程的播客节目数据结构
type PodcastEpisode struct {
	Title       string // 文章标题
	Summary     string // 文章摘要/NotebookLM 生成的内容
	SourceURL   string // 原文链接
	AudioPath   string // 本地音频文件路径
	AudioURL    string // R2 公开访问 URL
	AudioSize   int64  // 音频文件大小 (bytes)
	GeneratedAt time.Time
}

func main() {
	// 0. Load Env
	_ = godotenv.Load()

	// FIX: Override Proxy for Local Windows Execution
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:10809")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:10809")

	authToken := os.Getenv("NLM_AUTH_TOKEN")
	cookies := os.Getenv("NLM_COOKIES")
	if authToken == "" {
		log.Fatal("NLM_AUTH_TOKEN not set")
	}

	// 初始化 Episode 数据结构
	episode := &PodcastEpisode{
		SourceURL:   "https://mp.weixin.qq.com/s/rob9rTMJHFmZpkQkoLBZnQ",
		GeneratedAt: time.Now(),
	}

	// ========== [1/8] Fetch WeChat Article ==========
	fmt.Printf("[1/8] Fetching URL: %s ...\n", episode.SourceURL)
	html := fetchHTML(episode.SourceURL)

	// 尝试提取标题
	episode.Title = extractTitle(html)
	if episode.Title == "" {
		episode.Title = fmt.Sprintf("WeChat Podcast %s", time.Now().Format("2006-01-02"))
	}
	fmt.Printf("   > 标题: %s\n", episode.Title)

	// ========== [2/8] Clean Content ==========
	fmt.Println("[2/8] Cleaning content...")
	text, err := cleaner.Clean(html)
	if err != nil {
		log.Fatalf("Clean failed: %v", err)
	}
	if len(text) < 100 {
		log.Fatalf("Extracted text too short, something wrong.")
	}
	episode.Summary = truncateText(text, 500) // 摘要取前 500 字
	fmt.Printf("   > Extracted %d chars of text.\n", len(text))

	// ========== [3/8] Init NotebookLM Client ==========
	fmt.Println("[3/8] Initializing NotebookLM Client...")
	client := api.New(authToken, cookies)
	client.SetUseDirectRPC(true)

	// ========== [4/8] Create Notebook ==========
	fmt.Println("[4/8] Creating new Notebook...")
	notebookTitle := "WeChat Podcast: " + time.Now().Format("15:04:05")
	notebook, err := client.CreateProject(notebookTitle, "🎙️")
	if err != nil {
		log.Fatalf("CreateProject failed: %v", err)
	}
	projectID := notebook.ProjectId
	fmt.Printf("   > Created Notebook ID: %s\n", projectID)

	// ========== [5/8] Add Source ==========
	fmt.Println("[5/8] Uploading content as source...")
	sourceID, err := client.AddSourceFromText(projectID, text, episode.Title)
	if err != nil {
		log.Fatalf("AddSourceFromText failed: %v", err)
	}
	fmt.Printf("   > Added Source ID: %s\n", sourceID)

	fmt.Println("   > Waiting 5s for source processing...")
	time.Sleep(5 * time.Second)

	// ========== [6/8] Generate Audio ==========
	fmt.Println("[6/8] Generating Audio Overview (中文播客)...")
	prompt := "请生成一段深入的中文播客对话。两位主持人（一男一女）用中文讨论这篇文章的核心内容，风格轻松自然。"
	_, err = client.CreateAudioOverview(projectID, prompt)
	if err != nil {
		log.Printf("CreateAudioOverview warning: %v", err)
	}

	// Poll for audio completion
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			overview, err := client.GetAudioOverview(projectID)
			if err != nil {
				fmt.Printf("   Wait... (err: %v)\n", err)
				continue
			}

			fmt.Printf("   > Processing... DataLen: %d\n", len(overview.AudioData))

			if len(overview.AudioData) > 0 {
				fmt.Println("\n✅ Audio Generation Complete!")

				// Save audio locally
				episode.AudioPath = fmt.Sprintf("podcast_%s.mp3", time.Now().Format("20060102-150405"))
				if len(overview.AudioData) > 4 && overview.AudioData[:4] == "http" {
					fmt.Printf("   > Downloading from URL...\n")
					downloadFile(episode.AudioPath, overview.AudioData)
				} else {
					fmt.Println("   > Saving audio data directly...")
					os.WriteFile(episode.AudioPath, []byte(overview.AudioData), 0644)
				}

				// Get file size
				if info, err := os.Stat(episode.AudioPath); err == nil {
					episode.AudioSize = info.Size()
				}
				goto UPLOAD
			}
		}
	}

UPLOAD:
	ctx := context.Background()

	// ========== [7/8] Upload to R2 ==========
	fmt.Println("[7/8] Uploading to Cloudflare R2...")

	// Check if R2 is configured
	if os.Getenv("R2_ACCOUNT_ID") == "" {
		fmt.Println("   ⚠️ R2 not configured. Skipping upload and RSS update.")
		fmt.Printf("   > Local audio file: %s\n", episode.AudioPath)
		return
	}

	r2, err := uploader.NewR2Uploader()
	if err != nil {
		log.Printf("R2 初始化失败: %v", err)
		fmt.Printf("   > Local audio file: %s\n", episode.AudioPath)
		return
	}

	objectKey := fmt.Sprintf("episodes/%s", episode.AudioPath)
	episode.AudioURL, err = r2.UploadFile(ctx, episode.AudioPath, objectKey)
	if err != nil {
		log.Printf("上传失败: %v", err)
		fmt.Printf("   > Local audio file: %s\n", episode.AudioPath)
		return
	}
	fmt.Printf("   > 上传成功: %s\n", episode.AudioURL)

	// ========== [8/8] Update RSS Feed ==========
	fmt.Println("[8/8] Updating RSS Feed...")

	newItem := uploader.Item{
		Title:          episode.Title,
		Description:    episode.Summary,
		PubDate:        time.Now().Format(time.RFC1123Z),
		Guid:           episode.AudioURL,
		ItunesExplicit: "no",
		Enclosure: uploader.Enclosure{
			URL:    episode.AudioURL,
			Length: episode.AudioSize,
			Type:   "audio/mpeg",
		},
	}

	err = r2.UpdateRSS(ctx, "feed.xml", newItem)
	if err != nil {
		log.Printf("RSS 更新失败: %v", err)
	} else {
		fmt.Printf("   > RSS Feed: %s/feed.xml\n", os.Getenv("R2_PUBLIC_URL"))
	}

	// Clean up local file
	os.Remove(episode.AudioPath)

	fmt.Println("\n🎉 全部完成!")
	fmt.Printf("📻 播客地址: %s\n", episode.AudioURL)
	fmt.Printf("📡 RSS 订阅: %s/feed.xml\n", os.Getenv("R2_PUBLIC_URL"))
}

// ================= Helper Functions =================

func fetchHTML(url string) string {
	transport := &http.Transport{
		Proxy: nil,
	}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	fmt.Printf("   > Saved audio to %s\n", filepath)
	return err
}

// extractTitle 从 HTML 中提取文章标题
func extractTitle(html string) string {
	// 尝试匹配 <title>...</title>
	re := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := matches[1]
		// 移除微信默认的后缀
		re2 := regexp.MustCompile(`\s*[-–—]\s*微信公众平台$`)
		title = re2.ReplaceAllString(title, "")
		return title
	}
	return ""
}

// truncateText 截取文本前 n 个字符
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return text
}
