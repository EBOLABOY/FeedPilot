package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/auth"
)

func main() {
	_ = godotenv.Load()

	cfg := auth.DefaultEnsureConfig()
	cfg.Debug = true
	cfg.WriteDotenv = true
	cfg.WriteNlmEnv = true

	creds, err := auth.EnsureCredentials(cfg)
	if err != nil {
		log.Fatalf("❌ Auth refresh failed: %v", err)
	}

	fmt.Printf("✅ Credentials OK. TokenLen=%d CookiesLen=%d\n", len(creds.AuthToken), len(creds.Cookies))
}
