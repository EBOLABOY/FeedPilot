package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
	"notebook-podcast-automator/internal/batchexecute"
)

// Server 模拟一个长期运行的后端服务
type Server struct {
	client   *api.Client
	clientMu sync.RWMutex
	tokenPre string
}

func main() {
	_ = godotenv.Load()

	srv := &Server{}

	fmt.Println("🚀 [Server] Starting up...")
	if err := srv.refreshClient(); err != nil {
		log.Fatalf("Fatal: Could not initialize client: %v", err)
	}

	// 4. 启动 HTTP 服务
	http.HandleFunc("/status", srv.HandleStatus)
	http.HandleFunc("/generate", srv.HandleGenerate) // 假设这是一个触发生成的接口

	port := "8080"
	fmt.Printf("📡 [Server] Listening on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func (s *Server) refreshClient() error {
	creds, err := auth.EnsureCredentials(auth.DefaultEnsureConfig())
	if err != nil {
		return err
	}

	client := api.New(creds.AuthToken, creds.Cookies)

	s.clientMu.Lock()
	s.client = client
	s.tokenPre = tokenPrefix(creds.AuthToken, 10)
	s.clientMu.Unlock()

	return nil
}

func tokenPrefix(token string, n int) string {
	if token == "" || n <= 0 {
		return ""
	}
	if len(token) <= n {
		return token
	}
	return token[:n]
}

// HandleStatus 检查状态
func (s *Server) HandleStatus(w http.ResponseWriter, r *http.Request) {
	s.clientMu.RLock()
	clientAvailable := s.client != nil
	token := s.tokenPre
	s.clientMu.RUnlock()

	fmt.Fprintf(w, "Server Status: Running\n")
	fmt.Fprintf(w, "Client Available: %v\n", clientAvailable)
	if token != "" {
		fmt.Fprintf(w, "Current Token (prefix): %s...\n", token)
	}
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
		// Best-effort re-auth retry on 401
		if errors.Is(err, batchexecute.ErrUnauthorized) {
			if refreshErr := s.refreshClient(); refreshErr == nil {
				s.clientMu.RLock()
				client = s.client
				s.clientMu.RUnlock()
				projects, err = client.ListRecentlyViewedProjects()
			}
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("API Error: %v", err), 500)
			return
		}
	}

	fmt.Fprintf(w, "Successfully listed %d projects using the current live token.\n", len(projects))
}
