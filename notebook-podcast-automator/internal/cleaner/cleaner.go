package cleaner

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Clean extracts the main content from HTML and removes noise.
func Clean(htmlContent string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", err
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

	// Debug info (optional, can be removed in prod)
	// fmt.Printf("DEBUG: Extracted raw text length: %d\n", len(text))

	return cleanText(text), nil
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

	// Very specific noise phrases, not just single words
	noisePhrases := []string{
		"微信扫一扫", "关注该公众号", "轻触阅读原文",
		"预览时标签不可点", "只能预览", "相关阅读",
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip purely decorative lines
		if len([]rune(line)) < 2 {
			// Keep if it looks like a punctuation or bullet point? No, safe to skip.
			continue
		}

		// Exact match filtering (for button text etc)
		if line == "在看" || line == "点赞" || line == "分享" || line == "收藏" {
			continue
		}

		// Prefix filtering
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
