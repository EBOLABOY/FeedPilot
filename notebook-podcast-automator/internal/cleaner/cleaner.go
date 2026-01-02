package cleaner

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractContent fetches a URL, extracts title and cleans content.
func ExtractContent(url string) (string, string, error) {
	html, err := fetchHTML(url)
	if err != nil {
		return "", "", err
	}

	title := extractTitle(html)

	content, err := Clean(html)
	if err != nil {
		return title, "", err
	}
	if strings.TrimSpace(content) == "" {
		return title, "", fmt.Errorf("empty content after cleaning")
	}

	return title, content, nil
}

func fetchHTML(url string) (string, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if shouldBypassProxy(url) {
		transport.Proxy = nil
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	req, _ := http.NewRequest("GET", url, nil)
	// Use a common browser User-Agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func shouldBypassProxy(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" || host == "localhost" {
		return true
	}

	// WeChat is often blocked/unstable when routed via generic proxies; fetch directly.
	if strings.HasSuffix(host, "weixin.qq.com") {
		return true
	}

	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}

	return false
}

func extractTitle(html string) string {
	re := regexp.MustCompile(`<title[^>]*>([^<]+)</title>`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		title := matches[1]
		// Remove WeChat suffix
		re2 := regexp.MustCompile(`\s*[-–—]\s*微信公众平台$`)
		return re2.ReplaceAllString(title, "")
	}
	return ""
}

// Clean extracts the main content from HTML and removes noise.
func Clean(htmlContent string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
	}

	// WeChat sometimes returns a "share notice" wrapper (e.g. deleted/violated content),
	// where #js_content exists but contains no actual article text.
	if doc.Find("#js_content.share_notice_wrp").Length() > 0 {
		return "", fmt.Errorf("wechat content unavailable (share notice)")
	}

	// 1. Extract Main Content
	var content *goquery.Selection
	wechatSelectors := []string{"#js_content", ".rich_media_content"}
	for _, sel := range wechatSelectors {
		s := doc.Find(sel)
		if s.Length() > 0 {
			content = s
			break
		}
	}

	if content == nil {
		// Fallback...
		content = doc.Find("body")
	}

	if content == nil {
		return "", nil
	}

	// 2. Remove Noise
	removeNoise(content)

	// 3. Extract Text
	text := content.Text()
	cleaned := cleanText(text)
	if strings.TrimSpace(cleaned) == "" {
		return "", fmt.Errorf("empty content after cleaning")
	}
	return cleaned, nil
}

func removeNoise(s *goquery.Selection) {
	noiseTags := []string{"script", "style", "iframe", "noscript", "nav", "header", "footer", "form", "button"}
	for _, tag := range noiseTags {
		s.Find(tag).Remove()
	}

	// Conservative removal by class/id
	noiseClasses := []string{
		".qr-code", ".profile", ".recommend", ".tips",
		".reward_area", ".js_related",
	}
	for _, cls := range noiseClasses {
		s.Find(cls).Remove()
	}
}

func cleanText(raw string) string {
	lines := strings.Split(raw, "\n")
	var cleanedLines []string

	noisePhrases := []string{
		"微信扫一扫", "关注该公众号", "轻触阅读原文",
		"预览时标签不可点", "只能预览", "相关阅读",
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len([]rune(line)) < 2 {
			continue
		}
		if line == "在看" || line == "点赞" || line == "分享" || line == "收藏" {
			continue
		}

		isNoise := false
		for _, phrase := range noisePhrases {
			if strings.Contains(line, phrase) {
				isNoise = true
				break
			}
		}
		if isNoise {
			continue
		}

		cleanedLines = append(cleanedLines, line)
	}

	return strings.Join(cleanedLines, "\n")
}
