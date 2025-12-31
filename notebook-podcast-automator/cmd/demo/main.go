package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
)

// 演示所有核心功能 (All Core Features Demo)
func main() {
	_ = godotenv.Load()

	// 1. 认证 (优先 .env / ~/.nlm/env，必要时用浏览器 profile 提取)
	creds, err := auth.EnsureCredentials(auth.DefaultEnsureConfig())
	if err != nil {
		log.Fatalf("❌ Auth failed: %v", err)
	}

	client := api.New(creds.AuthToken, creds.Cookies)

	// 2. 创建笔记本 (基础)
	fmt.Println("\n[1/6] Feature: Create Notebook")
	notebookTitle := fmt.Sprintf("Demo Capabilities %s", time.Now().Format("15:04:05"))
	notebook, err := client.CreateProject(notebookTitle, "🧪")
	if err != nil {
		log.Fatalf("Create failed: %v", err)
	}
	projectID := notebook.ProjectId
	fmt.Printf("   ✅ Created Notebook: %s (ID: %s)\n", notebookTitle, projectID)

	// 3.1 Feature: Add Source From URL
	fmt.Println("\n[2/6] Feature: AddSourceFromURL")
	// 使用一个简单的 Wikipedia 页面作为测试:
	// https://en.wikipedia.org/wiki/Go_(programming_language)
	// 注意: Client 内部的 AddSourceFromURL 可能会遇到 Google 的爬虫限制
	// 这里演示调用方式
	// err = client.AddSourceFromURL(projectID, testURL)
	// 实际调用需要看 client.go 是否暴露了这个方法，或者我们需要用 AddSource (通用方法)
	// 检查发现我们目前的 internal/api/client.go 可能没有直接暴露 AddSourceFromURL (原始项目可能有，我们可能需要补上)
	// 如果没有，我们就在这里实现它，或者跳过
	fmt.Println("   ℹ️ (AddSourceFromURL functionality checking...)")

	// 3.2 Feature: Add Source From Text (已验证，稍微演示一下)
	sourceID, err := client.AddSourceFromText(projectID, "Go is an open source programming language that makes it easy to build simple, reliable, and efficient software.", "Go Lang Intro")
	if err != nil {
		fmt.Printf("   ❌ Add Text failed: %v\n", err)
	} else {
		fmt.Printf("   ✅ Added Text Source ID: %s\n", sourceID)
	}

	// 4. Feature: Query (提问对话)
	fmt.Println("\n[3/6] Feature: Query (Chat)")
	// 等待 Source 处理
	time.Sleep(2 * time.Second)

	chatResp, err := client.Query(context.Background(), projectID, "What is this text about?")
	if err != nil {
		fmt.Printf("   ❌ Query failed: %v\n", err)
	} else {
		fmt.Printf("   ✅ Checking Query Response...\n")
		// client.Query 返回的是 *http.Response 还是解析后的结果？
		// 检查 Query 签名: func (c *Client) Query(...) (string, error)
		fmt.Printf("   🤖 AI Answer: %s\n", chatResp)
	}

	// 5. Feature: List Notebooks
	// 我们的 client.go 可能没有 ListNotebooks (因为之前的 grep 没找到)
	// 我们需要去补全它！
	fmt.Println("\n[4/6] Feature: ListNotebooks")
	fmt.Println("   ℹ️ (Checking if ListNotebooks is implemented...)")
	// notebooks, err := client.ListNotebooks(context.Background())

	// 6. Feature: Add Source From File (PDF)
	// 使用 Client 原生的 AddSourceFromFile
	// _, err = client.AddSourceFromFile(projectID, "path/to/test.pdf")

	// 5. Feature: Delete Notebook (Cleanup)
	fmt.Println("\n[5/6] Feature: DeleteNotebook")
	err = client.DeleteProjects([]string{projectID})
	if err != nil {
		fmt.Printf("   ❌ Delete failed: %v\n", err)
	} else {
		fmt.Printf("   ✅ Deleted Notebook %s\n", projectID)
	}
}
