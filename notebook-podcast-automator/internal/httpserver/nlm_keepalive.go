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
		// Optional self-heal: when cookie refresh redirects to login, trigger interactive browser auth flow.
		if strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_BROWSER_AUTH_ON_REFRESH_FAIL")), "true") {
			// Force EnsureCredentials to go through refresh path first, then browser fallback if needed.
			restoreValue, hadValue := os.LookupEnv("NLM_REFRESH_TOKEN")
			_ = os.Setenv("NLM_REFRESH_TOKEN", "true")
			defer func() {
				if hadValue {
					_ = os.Setenv("NLM_REFRESH_TOKEN", restoreValue)
				} else {
					_ = os.Unsetenv("NLM_REFRESH_TOKEN")
				}
			}()

			if _, ensureErr := auth.EnsureCredentials(cfg); ensureErr != nil {
				log.Printf("[nlm_keepalive] interactive re-auth failed: %v", ensureErr)
				return
			}
			log.Printf("[nlm_keepalive] interactive re-auth succeeded")
		}
		return
	}

	log.Printf("[nlm_keepalive] refreshed")
}
