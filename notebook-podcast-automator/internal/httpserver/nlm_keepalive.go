package httpserver

import (
	"log"
	"os"
	"strings"
	"time"

	"notebook-podcast-automator/internal/auth"
)

func (s *Server) startNlmKeepaliveIfConfigured() {
	raw := strings.TrimSpace(os.Getenv("NLM_KEEPALIVE_INTERVAL"))
	if raw == "" {
		return
	}

	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		log.Printf("[nlm_keepalive] invalid NLM_KEEPALIVE_INTERVAL=%q: expected Go duration like 12h/30m", raw)
		return
	}

	writeFiles := strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_KEEPALIVE_PERSIST")), "true") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_PERSIST_COOKIES")), "true")

	log.Printf("[nlm_keepalive] enabled interval=%s persist=%v", interval, writeFiles)

	go func() {
		s.runNlmKeepaliveOnce(writeFiles)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			s.runNlmKeepaliveOnce(writeFiles)
		}
	}()
}

func (s *Server) runNlmKeepaliveOnce(writeFiles bool) {
	cfg := auth.DefaultEnsureConfig()
	cfg.WriteDotenv = writeFiles
	cfg.WriteNlmEnv = writeFiles

	if _, err := auth.ForceRefreshFromWeb(cfg); err != nil {
		log.Printf("[nlm_keepalive] refresh failed: %v", err)
		return
	}

	log.Printf("[nlm_keepalive] refreshed")
}

