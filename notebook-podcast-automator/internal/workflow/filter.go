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
	"招教",
	"招考",
	"招聘",
	"岗位表",
	"资格审查",
	"准考证",
	"成绩查询",
	"查分",
	"分数线",
	"领证",
	"拟录取",
	"拟聘用",
	"录取名单",
	"公示",
	"公告",
	"招标",
	"中标",
	"采购",
	"征稿",
	"投稿",
	"征订",
	"目录",
	"索引",
	"选题指南",
	"免费领",
	"优惠",
	"扫码",
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
		// Hybrid is: title prefilter (cheap) -> title LLM quick filter (cheaper) -> LLM deep filter (semantic).
		// The title prefilter is also applied before extraction to reduce fetching, but we apply it
		// again here to catch cases where the extracted title differs from the feed title.
		kept, dropped := prefilterCandidatesByRules(sources, cfg, markDropped)
		if dropped > 0 {
			progress.Report("filter", fmt.Sprintf("prefiltered extracted titles: kept=%d dropped=%d", len(kept), dropped))
		}
		sources = kept

		if filtered, err := filterSourcesByLLMTitles(ctx, sources, cfg, progress, markDropped); err != nil {
			progress.Report("warn", fmt.Sprintf("llm title filter skipped: %v", err))
		} else {
			sources = filtered
		}
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

func filterSourcesByLLMTitles(ctx context.Context, sources []Source, cfg Config, progress ProgressFunc, markDropped func(Source, string)) ([]Source, error) {
	if len(sources) == 0 {
		return sources, nil
	}
	model := strings.TrimSpace(cfg.FilterLLMTitleModel)
	if model == "" {
		return sources, nil
	}

	meaningful := make([]Source, 0, len(sources))
	meaningfulIndex := make([]int, 0, len(sources))
	skipped := 0
	for i, src := range sources {
		t := strings.TrimSpace(src.Title)
		if t == "" || isLikelyPlaceholderTitle(t) {
			skipped++
			continue
		}
		meaningful = append(meaningful, src)
		meaningfulIndex = append(meaningfulIndex, i)
	}
	if len(meaningful) == 0 {
		progress.Report("filter", fmt.Sprintf("llm title filter skipped: no meaningful titles (kept=%d)", len(sources)))
		return sources, nil
	}

	apiKey := strings.TrimSpace(cfg.FilterLLMAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("llm title filter enabled but api key is empty; set NPA_FILTER_LLM_API_KEY or OPENAI_API_KEY")
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

	retries := cfg.FilterLLMRetries
	if retries < 0 {
		retries = 0
	}
	if retries > 10 {
		retries = 10
	}
	attempts := retries + 1

	progress.Report("filter", fmt.Sprintf("llm title filtering sources (count=%d skipped=%d model=%s attempts=%d timeout=%s)", len(meaningful), skipped, model, attempts, timeout))

	httpClient := &http.Client{Timeout: timeout}

	var decisions map[int]llmDecision
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			backoff := llmRetryBackoff(attempt - 1)
			progress.Report("warn", fmt.Sprintf("llm title retrying (attempt=%d/%d after=%s): %v", attempt, attempts, backoff, lastErr))
			if err := sleepWithContext(ctx, backoff); err != nil {
				return nil, err
			}
		}

		decisions, lastErr = llmDecideTitlesBatch(ctx, httpClient, endpoint, apiKey, model, meaningful)
		if lastErr == nil {
			break
		}
		if attempt == attempts || !isRetryableLLMError(lastErr) {
			if attempt == attempts && attempts > 1 {
				return nil, fmt.Errorf("llm title request failed after %d attempts: %w", attempts, lastErr)
			}
			return nil, lastErr
		}
	}

	keepFlags := make([]bool, len(sources))
	for i, src := range sources {
		t := strings.TrimSpace(src.Title)
		if t == "" || isLikelyPlaceholderTitle(t) {
			keepFlags[i] = true
		}
	}

	kept := make([]Source, 0, len(sources))
	dropped := 0
	for j, src := range meaningful {
		orig := meaningfulIndex[j]
		decision, ok := decisions[j]
		if !ok {
			progress.Report("warn", fmt.Sprintf("llm title decision missing (keep by default): index=%d title=%s", orig, strings.TrimSpace(src.Title)))
			keepFlags[orig] = true
			continue
		}
		if decision.Keep {
			keepFlags[orig] = true
			continue
		}
		if markDropped != nil {
			markDropped(src, "llm_title:"+strings.TrimSpace(decision.Reason))
		}
		progress.Report("filter", fmt.Sprintf("llm title dropped: index=%d title=%s reason=%s", orig, strings.TrimSpace(src.Title), strings.TrimSpace(decision.Reason)))
	}

	for i, src := range sources {
		if keepFlags[i] {
			kept = append(kept, src)
			continue
		}
		dropped++
	}

	progress.Report("filter", fmt.Sprintf("llm title filtered sources: kept=%d dropped=%d", len(kept), dropped))
	return kept, nil
}

func isLikelyPlaceholderTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return true
	}
	if !strings.HasPrefix(t, "Article ") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(t, "Article "))
	if len(rest) != 8 || rest[2] != ':' || rest[5] != ':' {
		return true
	}
	for _, idx := range []int{0, 1, 3, 4, 6, 7} {
		if rest[idx] < '0' || rest[idx] > '9' {
			return true
		}
	}
	return true
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
		"你是专业教育播客的“核心选品主编”，你的任务是筛选出能支撑起一期“兼具宏观视野与实操深度”的高质量播客素材。",
		"最终目标：为 NotebookLM 提供高信噪比语料，生成内容需让教育从业者感到“有格局”或“可落地”。",
		"",
		"【筛选标准（Keep Criteria）】：",
		"文章必须至少满足以下两类价值之一，方可保留（keep=true）：",
		"",
		"1. 宏观视角（Macro Perspective）：",
		"- 政策深读：不只是转发文件，而是深入解读政策背后的教育逻辑、未来趋势或制度影响（如“强国建设”、“教育数字化”的深度分析）。",
		"- 行业前瞻：探讨 AI 变革、教育哲学、国际比较等具有长远影响的议题。",
		"- 底层逻辑：探讨教育本质、文化根源或社会结构对教育的影响。",
		"",
		"2. 落地实操（Practical Experience）：",
		"- 一线案例：包含具体的教学场景、PBL 项目流程、班级管理细节（如“如何解决某个具体学生问题”）。",
		"- 方法论：可复制的教学工具、心理学应用技巧、具体的教研成果。",
		"- 真实复盘：一线教师或管理者的真实工作手记，包含成功经验或失败教训的反思。",
		"",
		"【剔除标准（Drop Criteria）】：",
		"满足以下任一维度的文章必须剔除（keep=false）：",
		"1. 纯情绪/礼仪：单纯的新年贺词、节日祝福、抒情散文（除非其中包含了深刻的年度总结与反思）。",
		"2. 行政噪音：纯粹的会议通知、名单公示、考试安排、书目索引（无详细书评）。",
		"3. 空洞说教：只有口号没有路径，或只有理论没有现实连接的“正确的废话”。",
		"",
		"【输出要求】：",
		"1. 必须对每一篇文章都给出判断（keep 或 drop）。如果不确定，倾向 keep=true。",
		"2. podcast_title 生成规则：",
		"1) 必须基于 keep=true 的文章生成。",
		"2) 核心目标：提炼出一个最具“穿透力”的主题，体现“宏观趋势”对“一线实操”的实际影响。",
		"3) 风格要求：像“得到”或“小宇宙”的热门单集标题，极具吸引力；拒绝平铺直叙，拒绝大杂烩；必须有观点感，最好能击中教育者的痛点或盲点。",
		"4) 负面约束（Strict Negative Constraints）：绝对禁止使用“汇总”“简报”“合集”“一览”“动态”“几则”“文章”等表示列表的词汇；禁止包含日期/时间（如“2025年”“12月”）；不要带引号或书名号。",
		"5) 格式限制：中文，8~30字。",
		"6) 异常处理：若 keep=true 的文章数量为 0，则 podcast_title 为空字符串（\"\"）。",
		"3. 输出格式：只输出 JSON（禁止输出代码块、解释文本）。",
		"格式：{\"podcast_title\":\"...\",\"decisions\":[{\"index\":0,\"keep\":true,\"reason\":\"...\"},{\"index\":1,\"keep\":false,\"reason\":\"...\"}]}",
		"4. reason：必须具体说明该文属于“宏观分析”还是“落地实操”，不超过 60 字。",
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

func llmDecideTitlesBatch(ctx context.Context, httpClient *http.Client, endpoint string, apiKey string, model string, sources []Source) (map[int]llmDecision, error) {
	system := strings.Join([]string{
		"你是教育类内容的“初选过滤器”，仅根据标题和 URL 快速筛选素材。",
		"你的用户偏好：“宏观教育趋势”与“一线实操经验”。",
		"",
		"【筛选逻辑】：",
		"1. 优先保留（Keep）：",
		"- 关键词命中：标题包含“解读”、“趋势”、“变革”、“反思”、“案例”、“实践”、“复盘”、“手记”、“策略”、“模型”等。",
		"- 宏观类：涉及国家政策解读、未来教育形态、教育哲学探讨。",
		"- 实操类：看起来像是一线教师的具体教学方法、项目式学习（PBL）、心理干预案例。",
		"- 不确定时：如果标题看起来有实质内容（非纯通知），倾向于 keep=true，交给正文深筛处理。",
		"",
		"2. 坚决剔除（Drop）：",
		"- 纯礼仪类：如“新年献词”、“节日快乐”、“致...的一封信”（除非标题暗示有重磅干货）。",
		"- 纯信息流：如“目录总览”、“往期回顾”、“报名通知”、“公示”、“放假安排”。",
		"- 纯宣传：某某领导出席某某会议（无实质议题透露）。",
		"",
		"【输出要求】：",
		"只输出 JSON（禁止输出代码块、解释文本）。",
		"格式：{\"decisions\":[{\"index\":0,\"keep\":true,\"reason\":\"...\"},{\"index\":1,\"keep\":false,\"reason\":\"...\"}]}",
		"reason 不超过 40 字。",
	}, "\n")

	var userContent strings.Builder
	for i, src := range sources {
		title := strings.TrimSpace(src.Title)
		u := strings.TrimSpace(src.URL)
		_, _ = fmt.Fprintf(&userContent, "=== Article %d ===\n标题：%s\nURL：%s\n\n", i, title, u)
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
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, httpClient.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	bodyStr := strings.TrimSpace(string(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &llmHTTPError{
			StatusCode:  resp.StatusCode,
			Status:      resp.Status,
			ContentType: resp.Header.Get("Content-Type"),
			Body:        truncateRunes(compactWhitespace(bodyStr), 2048),
		}
	}

	var parsed openAIChatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &llmResponseParseError{
			Err:         err,
			ContentType: resp.Header.Get("Content-Type"),
			Snippet:     truncateRunes(compactWhitespace(bodyStr), 256),
		}
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return nil, fmt.Errorf("llm error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}

	items, _, ok := parseLLMBatchResponse(parsed.Choices[0].Message.Content)
	if !ok {
		return nil, fmt.Errorf("llm title decisions not parseable")
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
	return out, nil
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
