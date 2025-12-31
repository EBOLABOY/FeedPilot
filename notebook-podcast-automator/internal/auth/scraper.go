package auth

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// GetTokenFromCookies 使用 Cookies 访问 NotebookLM 主页提取最新的 Auth Token 和 Build Label
func GetTokenFromCookies(cookies string) (string, string, error) {
	// 设置代理 (尝试读取环境变量，如果没有则使用默认)
	proxyAddr := os.Getenv("HTTP_PROXY")
	if proxyAddr == "" {
		proxyAddr = "http://127.0.0.1:10809" // 默认 fallback
	}

	proxyUrl, err := url.Parse(proxyAddr)
	if err != nil {
		return "", "", fmt.Errorf("invalid proxy url: %v", err)
	}

	transport := &http.Transport{
		Proxy:           http.ProxyURL(proxyUrl),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", "https://notebooklm.google.com/", nil)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", cookies)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("status code: %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// DEBUG: 查看我们到底抓到了什么页面？
	preview := bodyString
	if len(preview) > 300 {
		preview = preview[:300]
	}
	fmt.Printf("\n[Scraper Debug] Page Title/Start: %s\n", preview)

	var token string

	// 1:1 复刻 nlm_upstream 的正则表达式逻辑
	// 优先寻找 SNlM0e (通常是 at 参数的值)
	atPattern := regexp.MustCompile(`"SNlM0e":"([^"]+)"`)
	matches := atPattern.FindStringSubmatch(bodyString)
	if len(matches) > 1 {
		token = matches[1]
		fmt.Printf("   > Found Token via SNlM0e: [%s...]\n", token[:min(10, len(token))])
	} else {
		// 备选方案: FdrFJe
		fmt.Println("   ⚠️ SNlM0e not found, trying FdrFJe...")
		fdrPattern := regexp.MustCompile(`"FdrFJe":"([^"]+)"`)
		matches = fdrPattern.FindStringSubmatch(bodyString)
		if len(matches) > 1 {
			token = matches[1]
			fmt.Printf("   > Found Token via FdrFJe: [%s...]\n", token[:min(10, len(token))])
		}
	}

	if token == "" {
		return "", "", fmt.Errorf("could not find auth token (SNlM0e or FdrFJe) in page")
	}

	// 提取 Build Label (bl)
	blPattern := regexp.MustCompile(`"boq_labs-tailwind-frontend_([^"]+)"`)
	blMatches := blPattern.FindStringSubmatch(bodyString)
	var bl string
	if len(blMatches) > 1 {
		bl = "boq_labs-tailwind-frontend_" + blMatches[1]
		fmt.Printf("   > Detected Build Label: %s\n", bl)
	}

	// 如果没找到 BL，使用硬编码的作为 fallback (但更新到一个较新的值)
	if bl == "" {
		bl = "boq_labs-tailwind-frontend_20251220.00_p0"
	}

	return token, bl, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// UpdateEnvFile 将新 Token 写入 .env 文件
func UpdateEnvFile(token string) error {
	content, err := os.ReadFile(".env")
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "NLM_AUTH_TOKEN=") {
			lines[i] = fmt.Sprintf("NLM_AUTH_TOKEN=\"%s\"", token)
			found = true
			break
		}
	}

	if !found {
		lines = append(lines, fmt.Sprintf("NLM_AUTH_TOKEN=\"%s\"", token))
	}

	newContent := strings.Join(lines, "\n")
	return os.WriteFile(".env", []byte(newContent), 0644)
}
