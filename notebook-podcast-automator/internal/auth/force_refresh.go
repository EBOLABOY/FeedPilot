package auth

import (
	"fmt"
	"os"
	"strings"
)

// ForceRefreshFromWeb fetches a fresh SNlM0e token from https://notebooklm.google.com/
// using the current cookies and returns updated credentials.
//
// Unlike EnsureCredentials, this does not fall back to browser auth and will return
// an error if the web refresh fails (e.g. cookies already expired and redirect to ServiceLogin).
func ForceRefreshFromWeb(cfg EnsureConfig) (Credentials, error) {
	token, cookies := loadFromProcessEnv()

	// Read .env first (project-local override).
	if cfg.DotenvPath != "" {
		if kv, err := readEnvFileKV(cfg.DotenvPath); err == nil {
			if token == "" {
				token = strings.TrimSpace(kv["NLM_AUTH_TOKEN"])
			}
			if cookies == "" {
				cookies = strings.TrimSpace(kv["NLM_COOKIES"])
			}
			if cfg.ProfileName == "" {
				cfg.ProfileName = strings.TrimSpace(kv["NLM_BROWSER_PROFILE"])
			}
		}
	}

	// Then read ~/.nlm/env (nlm_upstream default).
	nlmEnvPath, _ := nlmEnvPath()
	if cfg.ReadNlmEnv && nlmEnvPath != "" {
		if kv, err := readEnvFileKV(nlmEnvPath); err == nil {
			if token == "" {
				token = strings.TrimSpace(kv["NLM_AUTH_TOKEN"])
			}
			if cookies == "" {
				cookies = strings.TrimSpace(kv["NLM_COOKIES"])
			}
			if cfg.ProfileName == "" {
				cfg.ProfileName = strings.TrimSpace(kv["NLM_BROWSER_PROFILE"])
			}
		}
	}

	cookies = strings.TrimSpace(cookies)
	if cookies == "" {
		return Credentials{}, fmt.Errorf("cookies required")
	}

	freshToken, refreshedCookies, err := fetchSNlM0eTokenFromWeb(cookies)
	if err != nil {
		return Credentials{}, err
	}

	token = strings.TrimSpace(freshToken)
	if token == "" {
		return Credentials{}, fmt.Errorf("refresh returned empty token")
	}

	if strings.TrimSpace(refreshedCookies) != "" {
		cookies = strings.TrimSpace(refreshedCookies)
	}

	_ = os.Setenv("NLM_AUTH_TOKEN", token)
	_ = os.Setenv("NLM_COOKIES", cookies)

	// Persist refreshed token/cookies (best-effort).
	if cfg.WriteDotenv && cfg.DotenvPath != "" {
		updates := map[string]string{"NLM_AUTH_TOKEN": token}
		if strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_PERSIST_COOKIES")), "true") {
			updates["NLM_COOKIES"] = cookies
		}
		_ = updateEnvFileKeys(cfg.DotenvPath, updates)
	}
	if cfg.WriteNlmEnv && nlmEnvPath != "" {
		profile := cfg.ProfileName
		if profile == "" {
			profile = "Default"
		}
		_ = writeNlmEnvFile(nlmEnvPath, token, cookies, profile)
	}

	return Credentials{AuthToken: token, Cookies: cookies}, nil
}

