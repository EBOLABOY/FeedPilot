package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Credentials struct {
	AuthToken string
	Cookies   string
}

type EnsureConfig struct {
	DotenvPath string

	// If true, read `~/.nlm/env` (written by nlm_upstream).
	ReadNlmEnv bool

	// If true, write `~/.nlm/env` after browser auth succeeds.
	WriteNlmEnv bool

	// If true, update DotenvPath (keep other keys intact).
	WriteDotenv bool

	// Browser auth behavior
	Debug          bool
	ProfileName    string
	TryAllProfiles bool
	TargetURL      string
}

func DefaultEnsureConfig() EnsureConfig {
	// Conservative defaults:
	// - read both .env and ~/.nlm/env
	// - write ~/.nlm/env (upstream-compatible)
	// - only update .env if it already exists
	dotenvPath := ".env"
	_, dotenvExistsErr := os.Stat(dotenvPath)

	cfg := EnsureConfig{
		DotenvPath:     dotenvPath,
		ReadNlmEnv:     true,
		WriteNlmEnv:    true,
		WriteDotenv:    dotenvExistsErr == nil,
		Debug:          strings.EqualFold(os.Getenv("NLM_DEBUG"), "true"),
		ProfileName:    strings.TrimSpace(os.Getenv("NLM_BROWSER_PROFILE")),
		TargetURL:      "https://notebooklm.google.com",
		TryAllProfiles: false,
	}

	// If no profile is provided, try all profiles by default (more robust).
	if cfg.ProfileName == "" {
		cfg.TryAllProfiles = true
	}

	return cfg
}

func EnsureCredentials(cfg EnsureConfig) (Credentials, error) {
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

	if cookies != "" {
		refreshMode := strings.ToLower(strings.TrimSpace(os.Getenv("NLM_REFRESH_TOKEN")))
		forceRefresh := refreshMode == "1" || refreshMode == "true" || refreshMode == "yes"
		disableRefresh := refreshMode == "0" || refreshMode == "false" || refreshMode == "no"
		browserAuthOnRefreshFail := strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_BROWSER_AUTH_ON_REFRESH_FAIL")), "true")

		shouldRefresh := forceRefresh || token == ""
		if !shouldRefresh && token != "" {
			if _, expiryTime, err := ParseAuthToken(token); err == nil {
				shouldRefresh = time.Until(expiryTime) < 5*time.Minute
			}
		}

		if shouldRefresh && !disableRefresh {
			if freshToken, refreshedCookies, err := fetchSNlM0eTokenFromWeb(cookies); err == nil && freshToken != "" {
				token = freshToken
				if strings.TrimSpace(refreshedCookies) != "" {
					cookies = strings.TrimSpace(refreshedCookies)
				}

				_ = os.Setenv("NLM_AUTH_TOKEN", token)
				_ = os.Setenv("NLM_COOKIES", cookies)

				// Persist the refreshed token (best-effort).
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
			} else if err != nil && browserAuthOnRefreshFail && strings.Contains(err.Error(), "ServiceLogin") {
				token = ""
				cookies = ""
			}
		}
	}

	if token != "" && cookies != "" {
		_ = os.Setenv("NLM_AUTH_TOKEN", token)
		_ = os.Setenv("NLM_COOKIES", cookies)
		return Credentials{AuthToken: token, Cookies: cookies}, nil
	}

	// Last resort: use upstream browser auth (chromedp + real profile cookies).
	ba := New(cfg.Debug)

	var authOpts []Option
	authOpts = append(authOpts, WithTargetURL(cfg.TargetURL))
	if cfg.TryAllProfiles {
		authOpts = append(authOpts, WithTryAllProfiles())
	} else if cfg.ProfileName != "" {
		authOpts = append(authOpts, WithProfileName(cfg.ProfileName))
	}

	token, cookies, err := ba.GetAuth(authOpts...)
	if err != nil {
		return Credentials{}, fmt.Errorf("browser auth failed: %w", err)
	}
	if token == "" || cookies == "" {
		return Credentials{}, fmt.Errorf("browser auth returned empty token/cookies")
	}

	_ = os.Setenv("NLM_AUTH_TOKEN", token)
	_ = os.Setenv("NLM_COOKIES", cookies)

	// Persist credentials (best-effort, don’t fail the whole flow).
	if cfg.WriteNlmEnv && nlmEnvPath != "" {
		profile := cfg.ProfileName
		if profile == "" {
			profile = "Default"
		}
		_ = writeNlmEnvFile(nlmEnvPath, token, cookies, profile)
	}
	if cfg.WriteDotenv && cfg.DotenvPath != "" {
		_ = updateEnvFileKeys(cfg.DotenvPath, map[string]string{
			"NLM_AUTH_TOKEN": token,
			"NLM_COOKIES":    cookies,
		})
	}

	return Credentials{AuthToken: token, Cookies: cookies}, nil
}

func loadFromProcessEnv() (token, cookies string) {
	token = strings.TrimSpace(os.Getenv("NLM_AUTH_TOKEN"))
	cookies = strings.TrimSpace(os.Getenv("NLM_COOKIES"))
	return token, cookies
}

func nlmEnvPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nlm", "env"), nil
}
