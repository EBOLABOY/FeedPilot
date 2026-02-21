package auth

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func readEnvFileKV(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	kv := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		raw = strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		kv[key] = unquoteEnvValue(raw)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return kv, nil
}

func writeNlmEnvFile(path, token, cookies, browserProfile string, extra ...map[string]string) error {
	// Mirror nlm_upstream: overwrite with only the keys we control.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("NLM_COOKIES=%q\nNLM_AUTH_TOKEN=%q\nNLM_BROWSER_PROFILE=%q\n", cookies, token, browserProfile))

	if len(extra) > 0 && extra[0] != nil {
		keys := make([]string, 0, len(extra[0]))
		for k, v := range extra[0] {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("%s=%q\n", k, extra[0][k]))
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0600)
}

func updateEnvFileKeys(path string, updates map[string]string) error {
	origBytes, err := os.ReadFile(path)
	if err != nil {
		// If the file doesn't exist, create a new one.
		if os.IsNotExist(err) {
			var b strings.Builder
			for k, v := range updates {
				b.WriteString(fmt.Sprintf("%s=%q\n", k, v))
			}
			return os.WriteFile(path, []byte(b.String()), 0600)
		}
		return err
	}

	lines := strings.Split(string(origBytes), "\n")
	seen := make(map[string]bool, len(updates))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if val, exists := updates[key]; exists {
			lines[i] = fmt.Sprintf("%s=%q", key, val)
			seen[key] = true
		}
	}

	for key, val := range updates {
		if seen[key] {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%q", key, val))
	}

	// Preserve final newline behavior (simple + safe).
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out), 0600)
}

func unquoteEnvValue(raw string) string {
	if raw == "" {
		return ""
	}
	// Go-quoted string: "..."
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		if v, err := strconv.Unquote(raw); err == nil {
			return v
		}
		return strings.Trim(raw, "\"")
	}
	// Single-quoted: '...'
	if strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'") {
		return strings.Trim(raw, "'")
	}
	return raw
}
