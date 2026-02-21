package auth

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"notebook-podcast-automator/internal/batchexecute"
	"notebook-podcast-automator/internal/cookieutil"
)

var (
	snlm0eRe = regexp.MustCompile(`"SNlM0e"\s*:\s*"([^"]+)"`)
	fdrfjeRe = regexp.MustCompile(`"FdrFJe"\s*:\s*"([^"]+)"`)
	blRe     = regexp.MustCompile(`(boq_labs-tailwind-frontend_[A-Za-z0-9._-]+)`)
)

func fetchSNlM0eTokenFromWeb(cookies string) (token string, sessionID string, buildLabel string, refreshedCookies string, err error) {
	originalCookies := cookieutil.NormalizeCookieHeader(cookies)
	cookies = originalCookies
	if allowlist := strings.TrimSpace(os.Getenv("NLM_COOKIE_ALLOWLIST")); allowlist != "" {
		cookies, _ = cookieutil.FilterCookieHeader(cookies, allowlist)
		cookies = cookieutil.NormalizeCookieHeader(cookies)
	}
	if cookies == "" {
		return "", "", "", "", fmt.Errorf("cookies required")
	}

	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return "", "", "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	timeout := 30 * time.Second
	noRedirect := func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	proxyURL := strings.TrimSpace(os.Getenv("NLM_PROXY_URL"))
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(os.Getenv("HTTPS_PROXY"))
	}
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(os.Getenv("HTTP_PROXY"))
	}

	var clients []*http.Client
	if fpClient, fpErr := batchexecute.NewFingerprintedClient(timeout, proxyURL); fpErr == nil && fpClient != nil {
		fpClient.CheckRedirect = noRedirect
		clients = append(clients, fpClient)
	}
	stdClient := &http.Client{Timeout: timeout, CheckRedirect: noRedirect}
	clients = append(clients, stdClient)

	var lastErr error
	for _, client := range clients {
		resp, err := client.Do(req.Clone(req.Context()))
		if err != nil {
			lastErr = fmt.Errorf("fetch notebooklm: %w", err)
			continue
		}

		setCookieHeaders := resp.Header.Values("Set-Cookie")
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			location := strings.TrimSpace(resp.Header.Get("Location"))
			if location != "" {
				lastErr = fmt.Errorf("fetch notebooklm: unexpected status %s (redirect=%s)", resp.Status, location)
			} else {
				lastErr = fmt.Errorf("fetch notebooklm: unexpected status %s", resp.Status)
			}
			continue
		}
		if readErr != nil {
			lastErr = fmt.Errorf("read notebooklm page: %w", readErr)
			continue
		}

		parsedToken, parsedSessionID, parsedBuildLabel, parseErr := extractWebSessionFromHTML(body)
		if parseErr != nil {
			lastErr = parseErr
			continue
		}

		token = parsedToken
		sessionID = parsedSessionID
		buildLabel = parsedBuildLabel
		refreshedCookies, _ = cookieutil.MergeSetCookieHeaders(originalCookies, setCookieHeaders)
		return token, sessionID, buildLabel, refreshedCookies, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("fetch notebooklm: no HTTP client available")
	}
	return "", "", "", "", lastErr
}

func extractWebSessionFromHTML(body []byte) (token string, sessionID string, buildLabel string, err error) {
	m := snlm0eRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", "", "", fmt.Errorf("SNlM0e token not found in notebooklm page")
	}
	token = strings.TrimSpace(string(m[1]))
	if len(token) < 20 {
		return "", "", "", fmt.Errorf("SNlM0e token too short")
	}

	if sm := fdrfjeRe.FindSubmatch(body); len(sm) >= 2 {
		sessionID = strings.TrimSpace(string(sm[1]))
	}
	if bm := blRe.FindSubmatch(body); len(bm) >= 2 {
		buildLabel = strings.TrimSpace(string(bm[1]))
	}

	return token, sessionID, buildLabel, nil
}
