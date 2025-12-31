package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
)

// Server 模拟一个长期运行的后端服务
type Server struct {
	client   *api.Client
	clientMu sync.RWMutex
	tokenMgr *auth.Manager
}

func main() {
	_ = godotenv.Load()

	cookies := os.Getenv("NLM_COOKIES")
	initialToken := os.Getenv("NLM_AUTH_TOKEN") // 可以是空的，让 Manager 第一次去抓

	srv := &Server{}

	// 1. 初始化 TokenManager
	// 定义回调：当 Token 刷新时，更新 Server 里的 Client
	updateFunc := func(newToken string) {
		fmt.Printf("🔄 [Server] Detected token update. Re-initializing Client...\n")

		// 重新创建 Client 实例
		newClient := api.New(newToken, cookies)
		// 恢复之前的配置 (例如直连模式)
		newClient.SetUseDirectRPC(true)

		srv.clientMu.Lock()
		srv.client = newClient
		srv.clientMu.Unlock()

		fmt.Println("✨ [Server] Client updated with fresh token.")
	}

	srv.tokenMgr = auth.NewManager(cookies, initialToken, updateFunc)

	// 2. 也是第一次，确保我们有一个可用的 Token
	fmt.Println("🚀 [Server] Starting up...")
	if initialToken == "" {
		fmt.Println("   > No initial token, forcing refresh...")
		if err := srv.tokenMgr.Refresh(); err != nil {
			log.Fatalf("Fatal: Could not get initial token: %v", err)
		}
	} else {
		// 手动触发一次初始化 Client
		updateFunc(initialToken)
	}

	// 3. 启动后台保活 (每 45 分钟刷新一次)
	srv.tokenMgr.Start(45 * time.Minute)
	defer srv.tokenMgr.Stop()

	// 4. 启动 HTTP 服务
	http.HandleFunc("/status", srv.HandleStatus)
	http.HandleFunc("/generate", srv.HandleGenerate) // 假设这是一个触发生成的接口

	port := "8080"
	fmt.Printf("📡 [Server] Listening on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// HandleStatus 检查状态
func (s *Server) HandleStatus(w http.ResponseWriter, r *http.Request) {
	s.clientMu.RLock()
	clientAvailable := s.client != nil
	token := s.tokenMgr.GetToken()
	s.clientMu.RUnlock()

	fmt.Fprintf(w, "Server Status: Running\n")
	fmt.Fprintf(w, "Client Available: %v\n", clientAvailable)
	fmt.Fprintf(w, "Current Token (prefix): %s...\n", token[:10])
	fmt.Fprintf(w, "Time: %s\n", time.Now().Format(time.RFC3339))
}

// HandleGenerate 模拟业务请求
func (s *Server) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	s.clientMu.RLock()
	client := s.client
	s.clientMu.RUnlock()

	if client == nil {
		http.Error(w, "Client not ready", 503)
		return
	}

	// 这里可以调用 client.CreateProject...
	// 为了演示，我们只调用简单的 ListRecentlyViewedProjects
	fmt.Println("📨 [Server] Received request, calling NotebookLM API...")
	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		http.Error(w, fmt.Sprintf("API Error: %v", err), 500)
		return
	}

	fmt.Fprintf(w, "Successfully listed %d projects using the current live token.\n", len(projects))
}
