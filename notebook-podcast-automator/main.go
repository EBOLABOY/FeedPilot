package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
	"notebook-podcast-automator/internal/cleaner"
	"notebook-podcast-automator/internal/uploader"
)

// PodcastEpisode 贯穿全流程的播客节目数据结构
type PodcastEpisode struct {
	Title       string // 播客标题
	Summary     string // 播客摘要
	GeneratedAt time.Time
	AudioPath   string // 本地音频文件路径
	AudioURL    string // R2 公开访问 URL
	AudioSize   int64  // 音频文件大小 (bytes)
}

// SourceData 存储单篇文章的内容
type SourceData struct {
	Title   string
	Content string
}

func main() {
	_ = godotenv.Load()

	// FIX: Override Proxy for Local Windows Execution
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:10809")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:10809")

	// Get Input (URL or File)
	var inputTarget string
	if len(os.Args) > 1 {
		inputTarget = os.Args[1]
	} else {
		// Default test file if no args
		inputTarget = "test_feed.xml"
	}

	creds, err := auth.EnsureCredentials(auth.DefaultEnsureConfig())
	if err != nil {
		log.Fatalf("❌ Auth failed: %v", err)
	}
	cookies := creds.Cookies
	authToken := creds.AuthToken

	fmt.Printf("   > Debug: NLM_COOKIES loaded (length: %d)\n", len(cookies))

	// ========== Logic for Multi-Source Aggregation ==========
	var sources []SourceData
	var podcastTitle string

	// Determine if input is a URL or Local File
	var rssReader io.Reader
	var isRSS bool = false

	if strings.HasPrefix(inputTarget, "http") {
		// It's a remote URL.
		// Heuristic: ending in .xml OR user says it's RSS
		// For robustness, we check content start
		resp, err := http.Get(inputTarget)
		if err != nil {
			log.Fatalf("Failed to fetch input URL: %v", err)
		}
		defer resp.Body.Close()

		bodyBytes, _ := io.ReadAll(resp.Body)
		contentSample := string(bodyBytes)
		if len(contentSample) > 500 {
			contentSample = contentSample[:500]
		}

		if strings.Contains(contentSample, "<?xml") ||
			strings.Contains(contentSample, "<rss") ||
			strings.Contains(contentSample, "<feed") ||
			strings.HasSuffix(inputTarget, ".xml") {
			isRSS = true
			rssReader = strings.NewReader(string(bodyBytes))
		} else {
			// Failover to single article mode for non-XML URLs
			isRSS = false
		}
	} else if strings.HasSuffix(inputTarget, ".xml") {
		// Local file
		isRSS = true
		f, err := os.Open(inputTarget)
		if err != nil {
			log.Fatalf("Failed to open local file: %v", err)
		}
		defer f.Close()
		bodyBytes, _ := io.ReadAll(f)
		rssReader = strings.NewReader(string(bodyBytes))
	}

	if isRSS {
		// --- RSS Feed Mode ---
		fmt.Printf("[1/9] Detected RSS Feed: %s\n", inputTarget)
		podcastTitle = fmt.Sprintf("每日简报 %s", time.Now().Format("2006-01-02"))

		// Simple XML Parsing structures
		type Link struct {
			Href string `xml:"href,attr"`
		}
		type Entry struct {
			Title string `xml:"title"`
			Link  Link   `xml:"link"`
		}
		type Feed struct {
			Entries []Entry `xml:"entry"`
		}

		var feed Feed
		decoder := xml.NewDecoder(rssReader)
		if err := decoder.Decode(&feed); err != nil {
			log.Printf("XML Decode Warning: %v. (Might not be a standard Atom feed)", err)
		}

		fmt.Printf("   > Found %d entries. Processing top 3...\n", len(feed.Entries))
		count := 0
		for _, entry := range feed.Entries {
			if count >= 3 {
				break
			}
			url := entry.Link.Href
			if url == "" {
				continue
			}

			fmt.Printf("   > Scraping [%d]: %s\n", count+1, entry.Title)
			// Polite delay
			if count > 0 {
				time.Sleep(1 * time.Second)
			}

			// Now calling our enhanced cleaner.ExtractContent
			title, content, err := cleaner.ExtractContent(url)
			if err != nil {
				fmt.Printf("     ⚠️ Extractor error: %v\n", err)
				continue
			}
			sources = append(sources, SourceData{Title: title, Content: content})
			count++
		}
	} else {
		// --- Single Article Mode ---
		fmt.Printf("[1/9] Single Article Mode: %s\n", inputTarget)
		title, content, err := cleaner.ExtractContent(inputTarget)
		if err != nil {
			log.Fatalf("Extract failed: %v", err)
		}
		podcastTitle = title
		sources = append(sources, SourceData{Title: title, Content: content})
	}

	if len(sources) == 0 {
		log.Fatal("❌ No valid content found to process.")
	}

	// Init Episode
	episode := &PodcastEpisode{
		Title:       podcastTitle,
		GeneratedAt: time.Now(),
	}
	// Generate Summary
	if len(sources) == 1 {
		episode.Summary = fmt.Sprintf("Article: %s", sources[0].Title)
	} else {
		episode.Summary = fmt.Sprintf("Digest containing %d articles: ", len(sources))
		for i, s := range sources {
			if i < 3 {
				episode.Summary += s.Title + "; "
			}
		}
	}

	// ========== [3/9] Init NotebookLM Client ==========
	fmt.Println("[2/9] Initializing NotebookLM Client...")
	client := api.New(authToken, cookies)
	// client.SetUseDirectRPC(true) -> Removed to use standard orchestration service (Proto)

	// ========== [4/9] Create Notebook ==========
	fmt.Println("[3/9] Creating new Notebook...")
	// Use title as project name + emoji
	notebook, err := client.CreateProject(episode.Title, "🗞️")
	if err != nil {
		log.Fatalf("CreateProject failed: %v", err)
	}
	projectID := notebook.ProjectId
	fmt.Printf("   > Created Notebook ID: %s\n", projectID)

	// ========== [5/9] Add Sources ==========
	fmt.Println("[4/9] Uploading sources...")
	for i, src := range sources {
		fmt.Printf("   > Uploading source %d/%d: %s...\n", i+1, len(sources), src.Title)
		_, err := client.AddSourceFromText(projectID, src.Content, src.Title)
		if err != nil {
			fmt.Printf("     ⚠️ Upload failed: %v\n", err)
		}
		// Small delay to prevent rate limit
		time.Sleep(1 * time.Second)
	}

	// ========== [6/9] Generate Audio ==========
	fmt.Println("[5/9] Generating Audio Overview (中文播客)...")
	prompt := "请生成一段深入的中文播客对话。两位主持人（一男一女）用中文讨论这些文章的核心内容，风格轻松自然，寻找它们之间的联系。"
	_, err = client.CreateAudioOverview(projectID, prompt)
	if err != nil {
		log.Printf("CreateAudioOverview warning: %v", err)
	}

	// Poll for audio completion
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		overview, err := client.GetAudioOverview(projectID)
		if err != nil {
			fmt.Printf("   Wait... (err: %v)\n", err)
			continue
		}

		fmt.Printf("   > Processing... DataLen: %d\n", len(overview.AudioData))

		if len(overview.AudioData) > 0 {
			fmt.Println("\n✅ Audio Generation Complete!")

			// Save audio locally
			episode.AudioPath = fmt.Sprintf("podcast_%s.mp3", episode.GeneratedAt.Format("20060102-150405"))
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

UPLOAD:
	ctx := context.Background()

	// ========== [7/9] Upload to R2 ==========
	fmt.Println("[6/9] Uploading to Cloudflare R2...")

	// Check if R2 is configured
	if os.Getenv("R2_ACCOUNT_ID") == "" {
		fmt.Println("   ⚠️ R2 not configured. Skipping upload and RSS update.")
		fmt.Printf("   > Local audio file: %s\n", episode.AudioPath)
		return
	}

	r2, err := uploader.NewR2Uploader()
	if err != nil {
		log.Printf("R2 初始化失败: %v", err)
		return
	}

	objectKey := fmt.Sprintf("episodes/%s", episode.AudioPath)
	episode.AudioURL, err = r2.UploadFile(ctx, episode.AudioPath, objectKey)
	if err != nil {
		log.Printf("上传失败: %v", err)
		return
	}
	fmt.Printf("   > 上传成功: %s\n", episode.AudioURL)

	// ========== [8/9] Update RSS Feed ==========
	fmt.Println("[7/9] Updating RSS Feed...")

	newItem := uploader.Item{
		Title:          episode.Title,
		Description:    episode.Summary,
		PubDate:        episode.GeneratedAt.Format(time.RFC1123Z),
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

	// ========== [9/9] Cleanup ==========
	fmt.Println("[8/9] Cleaning up Notebook...")
	if err := client.DeleteProjects([]string{projectID}); err != nil {
		log.Printf("⚠️ Failed to delete notebook: %v", err)
	} else {
		fmt.Println("   ✅ Temporary notebook deleted.")
	}

	os.Remove(episode.AudioPath)
	fmt.Println("\n🎉 全部完成!")
	fmt.Printf("📻 播客地址: %s\n", episode.AudioURL)
}

// ================= Helper Functions =================

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
