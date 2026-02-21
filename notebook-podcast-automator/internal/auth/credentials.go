package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	// Keep browser open for manual login when interactive re-auth is needed.
	KeepOpenSeconds int
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
		KeepOpenSeconds: func() int {
			raw := strings.TrimSpace(os.Getenv("NLM_BROWSER_KEEP_OPEN_SECONDS"))
			if raw == "" {
				return 0
			}
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return 0
			}
			return n
		}(),
	}

	// If no profile is provided, try all profiles by default (more robust).
	if cfg.ProfileName == "" {
		cfg.TryAllProfiles = true
	}

	return cfg
}

func EnsureCredentials(cfg EnsureConfig) (Credentials, error) {
	token, cookies := loadFromProcessEnv()
	sessionID := strings.TrimSpace(os.Getenv("NLM_F_SID"))
	buildLabel := strings.TrimSpace(os.Getenv("NLM_BL"))

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
			if sessionID == "" {
				sessionID = strings.TrimSpace(kv["NLM_F_SID"])
			}
			if buildLabel == "" {
				buildLabel = strings.TrimSpace(kv["NLM_BL"])
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
			if sessionID == "" {
				sessionID = strings.TrimSpace(kv["NLM_F_SID"])
			}
			if buildLabel == "" {
				buildLabel = strings.TrimSpace(kv["NLM_BL"])
			}
		}
	}

	setSessionMetaEnv(sessionID, buildLabel)

	if cookies != "" {
		refreshMode := strings.ToLower(strings.TrimSpace(os.Getenv("NLM_REFRESH_TOKEN")))
		forceRefresh := refreshMode == "1" || refreshMode == "true" || refreshMode == "yes"
		disableRefresh := refreshMode == "0" || refreshMode == "false" || refreshMode == "no"
		browserAuthOnRefreshFail := strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_BROWSER_AUTH_ON_REFRESH_FAIL")), "true")
		disableSessionRefresh := strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_DISABLE_WEB_SESSION_REFRESH")), "true")

		shouldRefresh := forceRefresh || token == ""
		if !shouldRefresh && token != "" {
			if _, expiryTime, err := ParseAuthToken(token); err == nil {
				shouldRefresh = time.Until(expiryTime) < 5*time.Minute
			} else {
				// Token format changed or is stale. Force a web refresh for compatibility.
				shouldRefresh = true
			}
		}

		shouldSyncSession := !disableSessionRefresh

		if shouldSyncSession || (shouldRefresh && !disableRefresh) {
			if freshToken, freshSessionID, freshBuildLabel, refreshedCookies, err := fetchSNlM0eTokenFromWeb(cookies); err == nil && freshToken != "" {
				if !disableRefresh || token == "" {
					token = freshToken
				}
				if strings.TrimSpace(refreshedCookies) != "" {
					cookies = strings.TrimSpace(refreshedCookies)
				}
				if sid := strings.TrimSpace(freshSessionID); sid != "" {
					sessionID = sid
				}
				if bl := strings.TrimSpace(freshBuildLabel); bl != "" {
					buildLabel = bl
				}

				setSessionMetaEnv(sessionID, buildLabel)

				_ = os.Setenv("NLM_AUTH_TOKEN", token)
				_ = os.Setenv("NLM_COOKIES", cookies)

				// Persist refreshed auth/session metadata (best-effort).
				if cfg.WriteDotenv && cfg.DotenvPath != "" {
					updates := map[string]string{}
					if token != "" {
						updates["NLM_AUTH_TOKEN"] = token
					}
					if strings.EqualFold(strings.TrimSpace(os.Getenv("NLM_PERSIST_COOKIES")), "true") {
						updates["NLM_COOKIES"] = cookies
					}
					updates = addSessionMetaUpdates(updates, sessionID, buildLabel)
					if len(updates) > 0 {
						_ = updateEnvFileKeys(cfg.DotenvPath, updates)
					}
				}
				if cfg.WriteNlmEnv && nlmEnvPath != "" {
					profile := cfg.ProfileName
					if profile == "" {
						profile = "Default"
					}
					_ = writeNlmEnvFile(nlmEnvPath, token, cookies, profile, addSessionMetaUpdates(nil, sessionID, buildLabel))
				}
			} else if shouldRefresh && err != nil && browserAuthOnRefreshFail && strings.Contains(err.Error(), "ServiceLogin") {
				token = ""
				cookies = ""
			}
		}
	}

	if token != "" && cookies != "" {
		_ = os.Setenv("NLM_AUTH_TOKEN", token)
		_ = os.Setenv("NLM_COOKIES", cookies)
		setSessionMetaEnv(sessionID, buildLabel)
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
	if cfg.KeepOpenSeconds > 0 {
		authOpts = append(authOpts, WithKeepOpenSeconds(cfg.KeepOpenSeconds))
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

	// Best effort: extract dynamic session metadata from the live NotebookLM page.
	if freshToken, freshSessionID, freshBuildLabel, refreshedCookies, err := fetchSNlM0eTokenFromWeb(cookies); err == nil && freshToken != "" {
		token = freshToken
		if strings.TrimSpace(refreshedCookies) != "" {
			cookies = strings.TrimSpace(refreshedCookies)
		}
		if sid := strings.TrimSpace(freshSessionID); sid != "" {
			sessionID = sid
		}
		if bl := strings.TrimSpace(freshBuildLabel); bl != "" {
			buildLabel = bl
		}
	}

	_ = os.Setenv("NLM_AUTH_TOKEN", token)
	_ = os.Setenv("NLM_COOKIES", cookies)
	setSessionMetaEnv(sessionID, buildLabel)

	// Persist credentials (best-effort, don’t fail the whole flow).
	if cfg.WriteNlmEnv && nlmEnvPath != "" {
		profile := cfg.ProfileName
		if profile == "" {
			profile = "Default"
		}
		_ = writeNlmEnvFile(nlmEnvPath, token, cookies, profile, addSessionMetaUpdates(nil, sessionID, buildLabel))
	}
	if cfg.WriteDotenv && cfg.DotenvPath != "" {
		updates := addSessionMetaUpdates(map[string]string{
			"NLM_AUTH_TOKEN": token,
			"NLM_COOKIES":    cookies,
		}, sessionID, buildLabel)
		_ = updateEnvFileKeys(cfg.DotenvPath, updates)
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

func setSessionMetaEnv(sessionID, buildLabel string) {
	if sid := strings.TrimSpace(sessionID); sid != "" {
		_ = os.Setenv("NLM_F_SID", sid)
	}
	if bl := strings.TrimSpace(buildLabel); bl != "" {
		_ = os.Setenv("NLM_BL", bl)
	}
}

func addSessionMetaUpdates(updates map[string]string, sessionID, buildLabel string) map[string]string {
	if updates == nil {
		updates = make(map[string]string)
	}
	if sid := strings.TrimSpace(sessionID); sid != "" {
		updates["NLM_F_SID"] = sid
	}
	if bl := strings.TrimSpace(buildLabel); bl != "" {
		updates["NLM_BL"] = bl
	}
	return updates
}
