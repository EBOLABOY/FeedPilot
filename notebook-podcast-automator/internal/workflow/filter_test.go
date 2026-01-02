package workflow

import (
	"context"
	"testing"
)

func TestPrefilterCandidatesByRules_BlockKeywordInTitle(t *testing.T) {
	cfg := Config{
		FilterMode:          "rules",
		FilterBlockKeywords: []string{"报名", "考试"},
	}
	in := []Source{
		{Title: "2026 年考试报名通知", URL: "https://example.com/a"},
		{Title: "深度解读：教育改革与人才培养", URL: "https://example.com/b"},
	}

	out, dropped := prefilterCandidatesByRules(in, cfg, nil)
	if dropped != 1 {
		t.Fatalf("expected dropped=1, got %d", dropped)
	}
	if len(out) != 1 {
		t.Fatalf("expected kept=1, got %d", len(out))
	}
	if out[0].URL != "https://example.com/b" {
		t.Fatalf("unexpected kept url: %s", out[0].URL)
	}
}

func TestPrefilterCandidatesByRules_DefaultBlockKeywords(t *testing.T) {
	cfg := Config{
		FilterMode: "hybrid",
		// FilterBlockKeywords is nil: should use defaultBlockKeywords.
	}
	in := []Source{
		{Title: "扫码免费领：教学资料合集", URL: "https://example.com/a"},
		{Title: "深度解读：教育改革与人才培养", URL: "https://example.com/b"},
	}

	out, dropped := prefilterCandidatesByRules(in, cfg, nil)
	if dropped != 1 {
		t.Fatalf("expected dropped=1, got %d", dropped)
	}
	if len(out) != 1 {
		t.Fatalf("expected kept=1, got %d", len(out))
	}
	if out[0].URL != "https://example.com/b" {
		t.Fatalf("unexpected kept url: %s", out[0].URL)
	}
}

func TestFilterSourcesByRules_AllowOverridesBlock(t *testing.T) {
	cfg := Config{
		FilterMode:          "rules",
		FilterBlockKeywords: []string{"报名"},
		FilterAllowKeywords: []string{"深度"},
	}
	in := []Source{
		{Title: "深度：报名背后的教育资源配置问题", URL: "https://example.com/a", Content: "内容很长..."},
		{Title: "报名通知：XXX", URL: "https://example.com/b", Content: "内容很长..."},
	}

	out := filterSourcesByRules(in, cfg, nil, nil)
	if len(out) != 1 {
		t.Fatalf("expected kept=1, got %d", len(out))
	}
	if out[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected kept url: %s", out[0].URL)
	}
}

func TestFilterSourcesByRules_MinContentChars(t *testing.T) {
	cfg := Config{
		FilterMode:            "rules",
		FilterMinContentChars: 10,
	}
	in := []Source{
		{Title: "短文", URL: "https://example.com/a", Content: "太短"},
		{Title: "长文", URL: "https://example.com/b", Content: "这是一个足够长的正文内容"},
	}

	out := filterSourcesByRules(in, cfg, nil, nil)
	if len(out) != 1 {
		t.Fatalf("expected kept=1, got %d", len(out))
	}
	if out[0].URL != "https://example.com/b" {
		t.Fatalf("unexpected kept url: %s", out[0].URL)
	}
}

func TestParseLLMDecision_JSON(t *testing.T) {
	d, ok := parseLLMDecision("{\"keep\":false,\"reason\":\"报名通知\"}")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if d.Keep {
		t.Fatalf("expected keep=false")
	}
	if d.Reason != "报名通知" {
		t.Fatalf("unexpected reason: %q", d.Reason)
	}
}

func TestParseLLMBatchDecisions_JSONArray(t *testing.T) {
	items, ok := parseLLMBatchDecisions(`[{"index":0,"keep":true,"reason":"有观点"},{"index":1,"keep":false,"reason":"报名通知"}]`)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Index != 0 || !items[0].Keep {
		t.Fatalf("unexpected item0: %+v", items[0])
	}
	if items[1].Index != 1 || items[1].Keep {
		t.Fatalf("unexpected item1: %+v", items[1])
	}
}

func TestFilterSources_Hybrid_LLMMisconfiguredFallsBackToRules(t *testing.T) {
	cfg := Config{
		FilterMode:          "hybrid",
		FilterBlockKeywords: []string{"报名"},
		// Intentionally leave LLM config empty to force an error.
	}
	in := []Source{
		{Title: "报名通知：XXX", URL: "https://example.com/a", Content: "内容很长..."},
		{Title: "深度解读：教育改革与人才培养", URL: "https://example.com/b", Content: "内容很长..."},
	}

	out, _, err := filterSources(context.Background(), in, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected err=nil, got %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected kept=1, got %d", len(out))
	}
	if out[0].URL != "https://example.com/b" {
		t.Fatalf("unexpected kept url: %s", out[0].URL)
	}
}

func TestFilterSources_Hybrid_TitleLLMMisconfiguredFallsBackToRules(t *testing.T) {
	cfg := Config{
		FilterMode:          "hybrid",
		FilterBlockKeywords: []string{"报名"},
		FilterLLMTitleModel: "cheap-model",
		// Intentionally leave LLM config empty to force an error.
	}
	in := []Source{
		{Title: "报名通知：XXX", URL: "https://example.com/a", Content: "内容很长..."},
		{Title: "深度解读：教育改革与人才培养", URL: "https://example.com/b", Content: "内容很长..."},
	}

	out, _, err := filterSources(context.Background(), in, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected err=nil, got %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected kept=1, got %d", len(out))
	}
	if out[0].URL != "https://example.com/b" {
		t.Fatalf("unexpected kept url: %s", out[0].URL)
	}
}

func TestFilterSourcesByLLMTitles_AllPlaceholderTitlesSkipLLM(t *testing.T) {
	cfg := Config{
		FilterLLMTitleModel: "cheap-model",
		// Leave API key empty; should not error because we skip LLM when titles are placeholders.
	}
	in := []Source{
		{Title: "Article 11:54:58", URL: "https://example.com/a"},
		{Title: "Article 11:54:59", URL: "https://example.com/b"},
	}

	out, err := filterSourcesByLLMTitles(context.Background(), in, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected err=nil, got %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("expected kept=%d, got %d", len(in), len(out))
	}
}
