package auth

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var snlm0eRe = regexp.MustCompile(`"SNlM0e"\s*:\s*"([^"]+)"`)

func fetchSNlM0eTokenFromWeb(cookies string) (string, error) {
	cookies = strings.TrimSpace(cookies)
	if cookies == "" {
		return "", fmt.Errorf("cookies required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Cookie", cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch notebooklm: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch notebooklm: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("read notebooklm page: %w", err)
	}

	m := snlm0eRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", fmt.Errorf("SNlM0e token not found in notebooklm page")
	}
	token := string(m[1])
	if len(token) < 20 {
		return "", fmt.Errorf("SNlM0e token too short")
	}
	return token, nil
}
