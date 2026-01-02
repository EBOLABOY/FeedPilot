package workflow

import (
	"strings"
	"testing"
)

func TestWithDefaults_AudioPromptContainsBeijingTime(t *testing.T) {
	cfg := withDefaults(Config{})
	p := cfg.AudioPrompt
	if !strings.Contains(p, "当前北京时间：") {
		t.Fatalf("expected audio prompt to include 北京时间 prefix, got: %q", p)
	}
	if !strings.Contains(p, "UTC+8") {
		t.Fatalf("expected audio prompt to include UTC+8 hint, got: %q", p)
	}
	if !strings.Contains(p, "《预见未来》") {
		t.Fatalf("expected audio prompt to include show name, got: %q", p)
	}
}

