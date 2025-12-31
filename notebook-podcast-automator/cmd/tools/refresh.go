package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	cookiesRaw := os.Getenv("NLM_COOKIES")
	if cookiesRaw == "" {
		log.Fatal("Error: NLM_COOKIES is empty in .env")
	}

	fmt.Println("Attempting to fetch new token using existing cookies...")

	// 1. 设置代理
	proxyUrl, _ := url.Parse("http://127.0.0.1:10809")
	transport := &http.Transport{
		Proxy:           http.ProxyURL(proxyUrl),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}

	// 2. 构造请求
	req, _ := http.NewRequest("GET", "https://notebooklm.google.com/", nil)

	// 设置 Headers (模拟浏览器)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", cookiesRaw)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	// 3. 发送请求
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Fatalf("Login failed? StatusCode: %d", resp.StatusCode)
	}

	// 4. 读取响应并提取 Token
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// 正则匹配 SNlM0e":"..."
	re := regexp.MustCompile(`"SNlM0e":"([^"]+)"`)
	matches := re.FindStringSubmatch(bodyString)

	if len(matches) > 1 {
		newToken := matches[1]
		fmt.Printf("✅ Success! Found Token: %s...\n", newToken[:20])

		// 5. 更新 .env
		updateEnvFile(newToken)
	} else {
		// Log some body for debug
		fmt.Println("❌ Failed to find SNlM0e token in HTML.")
		fmt.Println("Preview of HTML (first 500 chars):")
		if len(bodyString) > 500 {
			fmt.Println(bodyString[:500])
		} else {
			fmt.Println(bodyString)
		}

		// Check if redirected to login
		if strings.Contains(bodyString, "accounts.google.com") {
			fmt.Println("⚠️ It seems we were redirected to the login page. Your cookies might be invalid.")
		}
	}
}

func updateEnvFile(newToken string) {
	content, err := os.ReadFile(".env")
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(content), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "NLM_AUTH_TOKEN=") {
			lines[i] = fmt.Sprintf("NLM_AUTH_TOKEN=\"%s\"", newToken)
			found = true
			break
		}
	}

	if !found {
		lines = append(lines, fmt.Sprintf("NLM_AUTH_TOKEN=\"%s\"", newToken))
	}

	newContent := strings.Join(lines, "\n")
	os.WriteFile(".env", []byte(newContent), 0644)
	fmt.Println("✅ Updated .env with new token.")
}
