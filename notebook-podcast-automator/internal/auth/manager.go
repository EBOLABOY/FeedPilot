package auth

import (
	"log"
	"os"
	"sync"
	"time"
)

// TokenUpdateFunc 是当 Token 更新时的回调函数签名
type TokenUpdateFunc func(newToken string)

// Manager 负责在后台维护和刷新 Token
type Manager struct {
	cookies        string
	currentToken   string
	mutex          sync.RWMutex
	updateCallback TokenUpdateFunc
	stopChan       chan struct{}
	ticker         *time.Ticker
}

// NewManager 创建一个新的 Token 管理器
// updateCallback: 可选，当 Token 刷新成功时调用此函数（用于更新 Client）
func NewManager(cookies string, initialToken string, callback TokenUpdateFunc) *Manager {
	return &Manager{
		cookies:        cookies,
		currentToken:   initialToken,
		updateCallback: callback,
		stopChan:       make(chan struct{}),
	}
}

// Start 启动后台刷新循环
// interval: 刷新间隔，建议 45-50 分钟
func (m *Manager) Start(interval time.Duration) {
	m.ticker = time.NewTicker(interval)

	go func() {
		log.Printf("[TokenManager] Started. Refresh interval: %v", interval)
		for {
			select {
			case <-m.ticker.C:
				m.Refresh()
			case <-m.stopChan:
				m.ticker.Stop()
				log.Println("[TokenManager] Stopped.")
				return
			}
		}
	}()
}

// Stop 停止后台刷新
func (m *Manager) Stop() {
	close(m.stopChan)
}

// Refresh 立即手动执行一次刷新
func (m *Manager) Refresh() error {
	log.Println("[TokenManager] refreshing token...")

	// Ignore BL here, manager mainly handles the auth token itself
	newToken, _, err := GetTokenFromCookies(m.cookies)
	if err != nil {
		log.Printf("[TokenManager] ⚠️ Refresh failed: %v", err)
		return err
	}

	m.mutex.Lock()
	m.currentToken = newToken
	m.mutex.Unlock()

	log.Println("[TokenManager] ✅ Token refreshed successfully!")

	// 1. 更新本地 .env 文件
	if err := UpdateEnvFile(newToken); err != nil {
		log.Printf("[TokenManager] Failed to update .env: %v", err)
	}

	// 2. 更新内存环境变量
	os.Setenv("NLM_AUTH_TOKEN", newToken)

	// 3. 触发回调（更新 Client）
	if m.updateCallback != nil {
		m.updateCallback(newToken)
	}

	return nil
}

// GetToken 获取当前最新的 Token (线程安全)
func (m *Manager) GetToken() string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.currentToken
}
