package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	filterModeNone   = "none"
	filterModeRules  = "rules"
	filterModeLLM    = "llm"
	filterModeHybrid = "hybrid"
)

var defaultBlockKeywords = []string{
	"报名",
	"考试",
	"招教",
	"招考",
	"招聘",
	"公示",
	"拟录取",
	"录取",
	"准考证",
	"成绩",
	"报名入口",
	"报名时间",
	"报名截止",
}

func normalizeFilterMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "off", "disable", "disabled", "false", "0", "none":
		return filterModeNone
	case "rule", "rules":
		return filterModeRules
	case "ai", "llm":
		return filterModeLLM
	case "hybrid", "mix", "mixed":
		return filterModeHybrid
	default:
		return filterModeNone
	}
}

func effectiveFilterBlockKeywords(cfg Config) []string {
	if cfg.FilterBlockKeywords != nil {
		return normalizeKeywords(cfg.FilterBlockKeywords)
	}
	if normalizeFilterMode(cfg.FilterMode) == filterModeRules || normalizeFilterMode(cfg.FilterMode) == filterModeHybrid {
		return normalizeKeywords(defaultBlockKeywords)
	}
	return nil
}

func normalizeKeywords(in []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(in))
	for _, raw := range in {
		kw := strings.TrimSpace(raw)
		if kw == "" {
			continue
		}
		kwLower := strings.ToLower(kw)
		if seen[kwLower] {
			continue
		}
		seen[kwLower] = true
		out = append(out, kw)
	}
	return out
}

func containsAnyKeyword(text string, keywords []string) (string, bool) {
	if len(keywords) == 0 {
		return "", false
	}
	t := strings.ToLower(text)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(t, strings.ToLower(kw)) {
			return kw, true
		}
	}
	return "", false
}

func prefilterCandidatesByRules(candidates []Source, cfg Config, markDropped func(Source, string)) (kept []Source, dropped int) {
	block := effectiveFilterBlockKeywords(cfg)
	allow := normalizeKeywords(cfg.FilterAllowKeywords)
	if len(block) == 0 && len(allow) == 0 {
		return candidates, 0
	}

	for _, c := range candidates {
		title := strings.TrimSpace(c.Title)
		if kw, ok := containsAnyKeyword(title, allow); ok {
			_ = kw
			kept = append(kept, c)
			continue
		}
		if kw, ok := containsAnyKeyword(title, block); ok {
			dropped++
			if markDropped != nil {
				markDropped(c, "title_block:"+strings.TrimSpace(kw))
			}
			continue
		}
		kept = append(kept, c)
	}
	return kept, dropped
}

func filterSources(ctx context.Context, sources []Source, cfg Config, progress ProgressFunc, markDropped func(Source, string)) ([]Source, string, error) {
	mode := normalizeFilterMode(cfg.FilterMode)
	if mode == filterModeNone {
		return sources, "", nil
	}

	llmTitle := ""

	if mode == filterModeRules {
		sources = filterSourcesByRules(sources, cfg, progress, markDropped)
	}
	if mode == filterModeHybrid {
		// Hybrid is: title prefilter (cheap) -> LLM deep filter (semantic).
		// The title prefilter is also applied before extraction to reduce fetching, but we apply it
		// again here to catch cases where the extracted title differs from the feed title.
		kept, dropped := prefilterCandidatesByRules(sources, cfg, markDropped)
		if dropped > 0 {
			progress.Report("filter", fmt.Sprintf("prefiltered extracted titles: kept=%d dropped=%d", len(kept), dropped))
		}
		sources = kept
	}
	if mode == filterModeLLM || mode == filterModeHybrid {
		filtered, title, err := filterSourcesByLLM(ctx, sources, cfg, progress, markDropped)
		if err != nil {
			if mode == filterModeLLM {
				return nil, "", err
			}
			progress.Report("warn", fmt.Sprintf("llm filter skipped: %v", err))
		} else {
			sources = filtered
			llmTitle = compactWhitespace(title)
			if llmTitle != "" {
				progress.Report("filter", fmt.Sprintf("llm suggested podcast title: %s", llmTitle))
			}
		}
	}

	if len(sources) == 0 {
		if cfg.FilterStrict {
			return nil, "", fmt.Errorf("no sources left after filtering")
		}
		progress.Report("filter", "all sources filtered out; noop")
		return nil, llmTitle, nil
	}

	return sources, llmTitle, nil
}

func filterSourcesByRules(sources []Source, cfg Config, progress ProgressFunc, markDropped func(Source, string)) []Source {
	block := effectiveFilterBlockKeywords(cfg)
	allow := normalizeKeywords(cfg.FilterAllowKeywords)
	minChars := cfg.FilterMinContentChars

	if len(block) == 0 && len(allow) == 0 && minChars <= 0 {
		return sources
	}

	kept := make([]Source, 0, len(sources))
	dropped := 0

	for _, s := range sources {
		title := strings.TrimSpace(s.Title)
		u := strings.TrimSpace(s.URL)
		content := strings.TrimSpace(s.Content)

		if kw, ok := containsAnyKeyword(title+" "+u, allow); ok {
			_ = kw
			kept = append(kept, s)
			continue
		}

		if minChars > 0 && runeLen(content) < minChars {
			dropped++
			if markDropped != nil {
				markDropped(s, "content_too_short")
			}
			continue
		}
		if kw, ok := containsAnyKeyword(title+" "+u, block); ok {
			_ = kw
			dropped++
			if markDropped != nil {
				markDropped(s, "rule_block:"+strings.TrimSpace(kw))
			}
			continue
		}
		if kw, ok := containsAnyKeyword(content, block); ok {
			_ = kw
			dropped++
			if markDropped != nil {
				markDropped(s, "rule_block_content:"+strings.TrimSpace(kw))
			}
			continue
		}
		kept = append(kept, s)
	}

	if dropped > 0 {
		progress.Report("filter", fmt.Sprintf("rules filtered sources: kept=%d dropped=%d", len(kept), dropped))
	}
	return kept
}

func runeLen(s string) int {
	return len([]rune(s))
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type llmHTTPError struct {
	StatusCode  int
	Status      string
	ContentType string
	Body        string
}

func (e *llmHTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("llm http %s", strings.TrimSpace(e.Status))
	}
	return fmt.Sprintf("llm http %s: %s", strings.TrimSpace(e.Status), body)
}

type llmResponseParseError struct {
	Err         error
	ContentType string
	Snippet     string
}

func (e *llmResponseParseError) Error() string {
	ct := strings.TrimSpace(e.ContentType)
	snippet := strings.TrimSpace(e.Snippet)
	switch {
	case ct != "" && snippet != "":
		return fmt.Sprintf("parse llm response: %v (contentType=%s snippet=%q)", e.Err, ct, snippet)
	case ct != "":
		return fmt.Sprintf("parse llm response: %v (contentType=%s)", e.Err, ct)
	case snippet != "":
		return fmt.Sprintf("parse llm response: %v (snippet=%q)", e.Err, snippet)
	default:
		return fmt.Sprintf("parse llm response: %v", e.Err)
	}
}

func (e *llmResponseParseError) Unwrap() error { return e.Err }

type llmDecision struct {
	Keep   bool    `json:"keep"`
	Reason string  `json:"reason,omitempty"`
	Score  float64 `json:"score,omitempty"`
}

func filterSourcesByLLM(ctx context.Context, sources []Source, cfg Config, progress ProgressFunc, markDropped func(Source, string)) ([]Source, string, error) {
	if len(sources) == 0 {
		return sources, "", nil
	}
	if strings.TrimSpace(cfg.FilterLLMModel) == "" {
		return nil, "", fmt.Errorf("llm filter enabled but model is empty; set NPA_FILTER_LLM_MODEL or pass filter_llm_model")
	}
	apiKey := strings.TrimSpace(cfg.FilterLLMAPIKey)
	if apiKey == "" {
		return nil, "", fmt.Errorf("llm filter enabled but api key is empty; set NPA_FILTER_LLM_API_KEY or OPENAI_API_KEY")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.FilterLLMBaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	endpoint := baseURL + "/chat/completions"

	timeout := cfg.FilterLLMTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxChars := cfg.FilterLLMMaxChars

	retries := cfg.FilterLLMRetries
	if retries < 0 {
		retries = 0
	}
	if retries > 10 {
		retries = 10
	}
	attempts := retries + 1

	progress.Report("filter", fmt.Sprintf("llm filtering sources (count=%d model=%s attempts=%d timeout=%s)", len(sources), cfg.FilterLLMModel, attempts, timeout))

	httpClient := &http.Client{Timeout: timeout}

	var decisions map[int]llmDecision
	var suggestedTitle string
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			backoff := llmRetryBackoff(attempt - 1)
			progress.Report("warn", fmt.Sprintf("llm retrying (attempt=%d/%d after=%s): %v", attempt, attempts, backoff, lastErr))
			if err := sleepWithContext(ctx, backoff); err != nil {
				return nil, "", err
			}
		}

		decisions, suggestedTitle, lastErr = llmDecideBatch(ctx, httpClient, endpoint, apiKey, cfg.FilterLLMModel, sources, maxChars)
		if lastErr == nil {
			break
		}
		if attempt == attempts || !isRetryableLLMError(lastErr) {
			if attempt == attempts && attempts > 1 {
				return nil, "", fmt.Errorf("llm request failed after %d attempts: %w", attempts, lastErr)
			}
			return nil, "", lastErr
		}
	}

	kept := make([]Source, 0, len(sources))
	dropped := 0
	for i, src := range sources {
		decision, ok := decisions[i]
		if !ok {
			progress.Report("warn", fmt.Sprintf("llm decision missing (keep by default): index=%d title=%s", i, strings.TrimSpace(src.Title)))
			kept = append(kept, src)
			continue
		}
		if decision.Keep {
			kept = append(kept, src)
			continue
		}
		dropped++
		if markDropped != nil {
			markDropped(src, "llm:"+strings.TrimSpace(decision.Reason))
		}
		progress.Report("filter", fmt.Sprintf("llm dropped: index=%d title=%s reason=%s", i, strings.TrimSpace(src.Title), strings.TrimSpace(decision.Reason)))
	}

	progress.Report("filter", fmt.Sprintf("llm filtered sources: kept=%d dropped=%d", len(kept), dropped))
	if len(kept) == 0 {
		suggestedTitle = ""
	}
	return kept, strings.TrimSpace(suggestedTitle), nil
}

type llmBatchDecision struct {
	Index  int    `json:"index"`
	Keep   bool   `json:"keep"`
	Reason string `json:"reason,omitempty"`
}

type llmBatchResponse struct {
	PodcastTitle string             `json:"podcast_title,omitempty"`
	Decisions    []llmBatchDecision `json:"decisions,omitempty"`
	Items        []llmBatchDecision `json:"items,omitempty"`
}

func llmDecideBatch(ctx context.Context, httpClient *http.Client, endpoint string, apiKey string, model string, sources []Source, maxChars int) (map[int]llmDecision, string, error) {
	system := strings.Join([]string{
		"你是中文内容筛选助手，用于将一组文章筛选为“适合做每日播客简报”的素材。",
		"过滤目标：剔除低信息密度内容，例如：报名/考试/招聘/公示/活动通知/广告营销/纯链接导流等；保留有观点、有事实、有分析、有深度的信息。",
		"必须对每一篇文章都给出判断（keep 或 drop）。如果不确定，倾向 keep=true。",
		"同时给出一个播客标题（podcast_title），用于作为音频文件名/播客标题，要求：中文、简洁、8~30字、不要包含日期/时间、不要带引号或书名号。",
		"podcast_title 必须仅根据 keep=true 的文章生成；禁止参考 keep=false 的文章。如果 keep=true 的文章数量为 0，则 podcast_title 必须为空字符串（\"\"）。",
		"只输出 JSON（禁止输出代码块、解释文本）。",
		"JSON 格式示例：{\"podcast_title\":\"...\",\"decisions\":[{\"index\":0,\"keep\":true,\"reason\":\"...\"},{\"index\":1,\"keep\":false,\"reason\":\"...\"}]}（当没有 keep=true 时，podcast_title 为空字符串）。",
		"reason 不超过 60 字。",
	}, "\n")

	var userContent strings.Builder
	for i, src := range sources {
		title := strings.TrimSpace(src.Title)
		u := strings.TrimSpace(src.URL)
		content := strings.TrimSpace(src.Content)
		excerpt := content
		if maxChars > 0 {
			excerpt = truncateRunes(content, maxChars)
		}
		_, _ = fmt.Fprintf(&userContent, "=== Article %d ===\n标题：%s\nURL：%s\n正文摘要：\n%s\n\n", i, title, u, excerpt)
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": userContent.String()},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", err
	}

	callCtx, cancel := context.WithTimeout(ctx, httpClient.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	bodyStr := strings.TrimSpace(string(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &llmHTTPError{
			StatusCode:  resp.StatusCode,
			Status:      resp.Status,
			ContentType: resp.Header.Get("Content-Type"),
			Body:        truncateRunes(compactWhitespace(bodyStr), 2048),
		}
	}

	var parsed openAIChatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", &llmResponseParseError{
			Err:         err,
			ContentType: resp.Header.Get("Content-Type"),
			Snippet:     truncateRunes(compactWhitespace(bodyStr), 256),
		}
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return nil, "", fmt.Errorf("llm error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, "", fmt.Errorf("llm response has no choices")
	}

	items, podcastTitle, ok := parseLLMBatchResponse(parsed.Choices[0].Message.Content)
	if !ok {
		return nil, "", fmt.Errorf("llm decisions not parseable")
	}

	out := make(map[int]llmDecision, len(sources))
	for _, item := range items {
		if item.Index < 0 || item.Index >= len(sources) {
			continue
		}
		d := llmDecision{Keep: item.Keep, Reason: strings.TrimSpace(item.Reason)}
		if d.Reason == "" {
			d.Reason = "未提供原因"
		}
		out[item.Index] = d
	}
	return out, compactWhitespace(podcastTitle), nil
}

func parseLLMDecision(s string) (llmDecision, bool) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return llmDecision{}, false
	}

	if strings.Contains(raw, "```") {
		raw = strings.ReplaceAll(raw, "```json", "```")
		raw = strings.ReplaceAll(raw, "```", "")
		raw = strings.TrimSpace(raw)
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var d llmDecision
	if err := json.Unmarshal([]byte(raw), &d); err == nil {
		return d, true
	}

	up := strings.ToUpper(strings.TrimSpace(s))
	if strings.HasPrefix(up, "KEEP") {
		return llmDecision{Keep: true, Reason: strings.TrimSpace(strings.TrimPrefix(s, "KEEP"))}, true
	}
	if strings.HasPrefix(up, "DROP") {
		return llmDecision{Keep: false, Reason: strings.TrimSpace(strings.TrimPrefix(s, "DROP"))}, true
	}
	return llmDecision{}, false
}

func parseLLMBatchDecisions(s string) ([]llmBatchDecision, bool) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, false
	}

	if strings.Contains(raw, "```") {
		raw = strings.ReplaceAll(raw, "```json", "```")
		raw = strings.ReplaceAll(raw, "```", "")
		raw = strings.TrimSpace(raw)
	}

	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var items []llmBatchDecision
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		return items, true
	}

	// Tolerant parse for slightly non-conforming outputs.
	if items, ok := parseLLMBatchDecisionsLoose(raw); ok {
		return items, true
	}

	var wrapped struct {
		Decisions []llmBatchDecision `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && len(wrapped.Decisions) > 0 {
		return wrapped.Decisions, true
	}

	var wrappedAny map[string]any
	if err := json.Unmarshal([]byte(raw), &wrappedAny); err == nil {
		if v, ok := wrappedAny["decisions"]; ok {
			if items, ok := parseLLMBatchDecisionsLoose(v); ok {
				return items, true
			}
		}
	}
	return nil, false
}

func parseLLMBatchResponse(s string) ([]llmBatchDecision, string, bool) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return nil, "", false
	}

	if strings.Contains(raw, "```") {
		raw = strings.ReplaceAll(raw, "```json", "```")
		raw = strings.ReplaceAll(raw, "```", "")
		raw = strings.TrimSpace(raw)
	}

	startObj := strings.Index(raw, "{")
	endObj := strings.LastIndex(raw, "}")
	if startObj >= 0 && endObj > startObj {
		obj := raw[startObj : endObj+1]
		var wrapped llmBatchResponse
		if err := json.Unmarshal([]byte(obj), &wrapped); err == nil {
			title := strings.TrimSpace(wrapped.PodcastTitle)
			items := wrapped.Decisions
			if len(items) == 0 {
				items = wrapped.Items
			}
			if len(items) > 0 {
				return items, title, true
			}
		}

		var wrappedAny map[string]any
		if err := json.Unmarshal([]byte(obj), &wrappedAny); err == nil {
			title, _ := wrappedAny["podcast_title"].(string)
			if v, ok := wrappedAny["decisions"]; ok {
				if items, ok := parseLLMBatchDecisionsLoose(v); ok {
					return items, strings.TrimSpace(title), true
				}
			}
			if v, ok := wrappedAny["items"]; ok {
				if items, ok := parseLLMBatchDecisionsLoose(v); ok {
					return items, strings.TrimSpace(title), true
				}
			}
		}
	}

	items, ok := parseLLMBatchDecisions(raw)
	if !ok {
		return nil, "", false
	}
	return items, "", true
}

func parseLLMBatchDecisionsLoose(input any) ([]llmBatchDecision, bool) {
	switch v := input.(type) {
	case string:
		var arr []map[string]any
		if err := json.Unmarshal([]byte(v), &arr); err != nil {
			return nil, false
		}
		return convertLooseDecisionMaps(arr)
	case []any:
		arr := make([]map[string]any, 0, len(v))
		for _, it := range v {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			arr = append(arr, m)
		}
		if len(arr) == 0 {
			return nil, false
		}
		return convertLooseDecisionMaps(arr)
	case []map[string]any:
		return convertLooseDecisionMaps(v)
	default:
		return nil, false
	}
}

func convertLooseDecisionMaps(arr []map[string]any) ([]llmBatchDecision, bool) {
	out := make([]llmBatchDecision, 0, len(arr))
	for _, m := range arr {
		idx, ok := anyToInt(m["index"])
		if !ok {
			continue
		}
		keep, ok := anyToBool(m["keep"])
		if !ok {
			// Allow alternate fields.
			if action, ok2 := m["action"]; ok2 {
				if b, ok3 := anyToBool(action); ok3 {
					keep = b
					ok = true
				}
				if s, ok3 := action.(string); ok3 {
					up := strings.ToUpper(strings.TrimSpace(s))
					if up == "KEEP" || up == "PASS" {
						keep = true
						ok = true
					}
					if up == "DROP" || up == "FILTER" {
						keep = false
						ok = true
					}
				}
			}
		}
		if !ok {
			continue
		}
		reason, _ := m["reason"].(string)
		out = append(out, llmBatchDecision{
			Index:  idx,
			Keep:   keep,
			Reason: reason,
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func anyToInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(x))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func anyToBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case float64:
		return x != 0, true
	case int:
		return x != 0, true
	case int64:
		return x != 0, true
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		switch s {
		case "1", "true", "yes", "y", "on", "keep", "pass":
			return true, true
		case "0", "false", "no", "n", "off", "drop", "filter":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func splitCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func compactWhitespace(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func llmRetryBackoff(retryIndex int) time.Duration {
	if retryIndex <= 0 {
		return 0
	}
	shift := retryIndex - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 4 {
		shift = 4
	}
	d := 2 * time.Second * time.Duration(1<<shift)
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var httpErr *llmHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
		return httpErr.StatusCode >= 500 && httpErr.StatusCode <= 599
	}

	var parseErr *llmResponseParseError
	if errors.As(err, &parseErr) {
		return true
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "tempor") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "overloaded") ||
		strings.Contains(msg, "unavailable") {
		return true
	}
	return false
}
