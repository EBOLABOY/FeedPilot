package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
	"notebook-podcast-automator/internal/batchexecute"
	"notebook-podcast-automator/internal/netutil"
)

func main() {
	// 1. 加载环境变量
	log.Println("1. Loading .env...")
	env, err := godotenv.Read()
	if err != nil {
		log.Printf("Warning: Failed to read .env file: %v", err)
	}
	cookies := env["NLM_COOKIES"]
	if cookies == "" {
		cookies = os.Getenv("NLM_COOKIES")
	}
	if cookies == "" {
		log.Fatal("❌ NLM_COOKIES not found in .env or environment")
	}
	log.Printf("   Cookies loaded (len: %d)", len(cookies))

	// 2. 设置代理 (如果需要)
	netutil.MaybeSetLocalProxy()

	// 3. 获取 Token
	log.Println("2. Ensuring NotebookLM credentials...")
	creds, err := auth.EnsureCredentials(auth.DefaultEnsureConfig())
	if err != nil {
		log.Fatalf("❌ Failed to get credentials: %v", err)
	}
	cookies = creds.Cookies
	authToken := creds.AuthToken
	log.Printf("   Token retrieved (len: %d)", len(authToken))

	// 4. 初始化客户端
	log.Println("3. Initializing Client (Proto/OR Service)...")

	clientOpts := []batchexecute.Option{
		batchexecute.WithHeaders(map[string]string{
			"x-goog-authuser": "0",
			"x-origin":        "https://notebooklm.google.com",
		}),
	}

	client := api.New(authToken, cookies, append(clientOpts, batchexecute.WithDebug(true))...)

	// 5. 测试 API 调用：列出最近的项目
	log.Println("4. Testing API: ListRecentlyViewedProjects...")
	projects, err := client.ListRecentlyViewedProjects() // 这本质上也是 ListNotebooks
	if err != nil {
		log.Fatalf("❌ API Call Failed: %v", err)
	}

	log.Printf("✅ Success! Found %d projects:", len(projects))
	for i, p := range projects {
		if i >= 5 {
			break
		}
		updateTime := "Unknown"
		if p.Metadata != nil && p.Metadata.ModifiedTime != nil {
			updateTime = p.Metadata.ModifiedTime.AsTime().Format(time.RFC3339)
		}
		fmt.Printf("   - [%s] %s (Updated: %s)\n", p.ProjectId, p.Title, updateTime)
	}
}
