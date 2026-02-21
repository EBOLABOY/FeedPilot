package httpserver

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"notebook-podcast-automator/internal/auth"
)

func (s *Server) bootstrapAuthIfConfigured() error {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_BOOTSTRAP_AUTH_ON_START")), "true") {
		return nil
	}

	markerPath := strings.TrimSpace(os.Getenv("NLM_BOOTSTRAP_MARKER_PATH"))
	if markerPath == "" {
		markerPath = filepath.Join("data", ".nlm_bootstrap_done")
	}
	requireSuccess := strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_BOOTSTRAP_AUTH_REQUIRE_SUCCESS")), "true")

	if st, err := os.Stat(markerPath); err == nil && !st.IsDir() {
		log.Printf("[bootstrap_auth] marker exists, skip bootstrap: %s", markerPath)
		return nil
	}

	log.Printf("[bootstrap_auth] running one-time bootstrap auth")

	restoreRefresh, hadRefresh := os.LookupEnv("NLM_REFRESH_TOKEN")
	_ = os.Setenv("NLM_REFRESH_TOKEN", "true")
	defer func() {
		if hadRefresh {
			_ = os.Setenv("NLM_REFRESH_TOKEN", restoreRefresh)
		} else {
			_ = os.Unsetenv("NLM_REFRESH_TOKEN")
		}
	}()

	if strings.TrimSpace(os.Getenv("NLM_BROWSER_AUTH_ON_REFRESH_FAIL")) == "" {
		_ = os.Setenv("NLM_BROWSER_AUTH_ON_REFRESH_FAIL", "true")
	}

	cfg := auth.DefaultEnsureConfig()
	cfg.WriteDotenv = true
	cfg.WriteNlmEnv = true

	if _, err := auth.EnsureCredentials(cfg); err != nil {
		if requireSuccess {
			return fmt.Errorf("bootstrap auth failed: %w", err)
		}
		log.Printf("[bootstrap_auth] bootstrap auth failed, continue without blocking: %v", err)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(markerPath), 0755); err != nil {
		log.Printf("[bootstrap_auth] create marker dir failed: %v", err)
		return nil
	}
	if err := os.WriteFile(markerPath, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0644); err != nil {
		log.Printf("[bootstrap_auth] write marker failed: %v", err)
		return nil
	}

	log.Printf("[bootstrap_auth] bootstrap auth completed, marker=%s", markerPath)
	return nil
}
