package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"notebook-podcast-automator/internal/api"
	"notebook-podcast-automator/internal/auth"
	"notebook-podcast-automator/internal/batchexecute"
	"notebook-podcast-automator/internal/cookieutil"
	"notebook-podcast-automator/internal/netutil"
)

type attempt struct {
	remove string
	ok     bool
	err    error
}

func main() {
	var dotenvPath string
	var timeoutSeconds int
	var delayMs int
	var forceRefresh bool
	var writeDotenv bool
	var startAllowlist string
	var useEnvAllowlist bool

	flag.StringVar(&dotenvPath, "dotenv", ".env", "path to .env file")
	flag.IntVar(&timeoutSeconds, "timeout", 60, "HTTP timeout in seconds")
	flag.IntVar(&delayMs, "delay-ms", 250, "delay between attempts (ms)")
	flag.BoolVar(&forceRefresh, "force-refresh", true, "force refresh SNlM0e token from web on every attempt")
	flag.BoolVar(&writeDotenv, "write-dotenv", true, "write final NLM_COOKIE_ALLOWLIST back to .env")
	flag.StringVar(&startAllowlist, "start", "", "override starting allowlist (comma/space separated; supports prefix* wildcard)")
	flag.BoolVar(&useEnvAllowlist, "use-env-allowlist", false, "use NLM_COOKIE_ALLOWLIST as starting set when -start is empty")
	flag.Parse()

	if dotenvPath != "" {
		_ = godotenv.Overload(dotenvPath)
	}
	netutil.MaybeSetLocalProxy()

	timeout := time.Duration(timeoutSeconds) * time.Second
	delay := time.Duration(delayMs) * time.Millisecond

	fullCookieHeader := strings.TrimSpace(os.Getenv("NLM_COOKIES"))
	if fullCookieHeader == "" {
		fmt.Fprintln(os.Stderr, "NLM_COOKIES is empty")
		os.Exit(2)
	}

	allCookieNames := cookieNames(fullCookieHeader)
	if len(allCookieNames) == 0 {
		fmt.Fprintln(os.Stderr, "failed to parse cookie names from NLM_COOKIES")
		os.Exit(2)
	}
	sort.Strings(allCookieNames)

	rawAllowlist := strings.TrimSpace(startAllowlist)
	if rawAllowlist == "" && useEnvAllowlist {
		rawAllowlist = strings.TrimSpace(os.Getenv("NLM_COOKIE_ALLOWLIST"))
	}

	var keep []string
	if strings.TrimSpace(rawAllowlist) == "" {
		keep = append([]string(nil), allCookieNames...)
	} else {
		parsed := cookieutil.ParseAllowlist(rawAllowlist)
		for _, name := range allCookieNames {
			if parsed.Allows(name) {
				keep = append(keep, name)
			}
		}
	}

	if len(keep) == 0 {
		fmt.Fprintln(os.Stderr, "allowlist expansion produced empty cookie set; refusing to continue")
		os.Exit(2)
	}

	fmt.Printf("[init] cookies_total=%d cookies_selected=%d\n", len(allCookieNames), len(keep))
	fmt.Printf("[init] selected=%s\n", strings.Join(keep, ","))

	if err := verify(keep, forceRefresh, timeout); err != nil {
		fmt.Fprintf(os.Stderr, "[init] FAIL: current selection cannot authenticate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[init] OK: baseline works\n")

	sort.Strings(keep)

	var removed []string
	var attempts []attempt

	for i := 0; i < len(keep); i++ {
		name := keep[i]
		candidate := removeOne(keep, name)
		if len(candidate) == 0 {
			continue
		}

		err := verify(candidate, forceRefresh, timeout)
		ok := err == nil
		attempts = append(attempts, attempt{remove: name, ok: ok, err: err})

		if ok {
			removed = append(removed, name)
			keep = candidate
			i = -1 // restart: removal can make more cookies redundant
		}

		if delay > 0 {
			time.Sleep(delay)
		}
	}

	sort.Strings(keep)
	sort.Strings(removed)

	fmt.Printf("[result] kept=%d removed=%d\n", len(keep), len(removed))
	if len(removed) > 0 {
		fmt.Printf("[result] removed=%s\n", strings.Join(removed, ","))
	}
	fmt.Printf("[result] allowlist=%s\n", strings.Join(keep, ","))

	if writeDotenv {
		if dotenvPath == "" {
			fmt.Fprintln(os.Stderr, "[write] dotenv path is empty; cannot write")
			os.Exit(2)
		}
		if err := updateEnvFileKey(dotenvPath, "NLM_COOKIE_ALLOWLIST", strings.Join(keep, ",")); err != nil {
			fmt.Fprintf(os.Stderr, "[write] FAIL: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[write] OK: updated %s\n", filepath.Clean(dotenvPath))
	}
}

func verify(allowlistNames []string, forceRefresh bool, timeout time.Duration) error {
	restore := stashEnv("NLM_COOKIE_ALLOWLIST", "NLM_REFRESH_TOKEN")
	defer restore()

	_ = os.Setenv("NLM_COOKIE_ALLOWLIST", strings.Join(allowlistNames, ","))
	if forceRefresh {
		_ = os.Setenv("NLM_REFRESH_TOKEN", "true")
	} else {
		_ = os.Unsetenv("NLM_REFRESH_TOKEN")
	}

	ensureCfg := auth.DefaultEnsureConfig()
	ensureCfg.WriteDotenv = false
	ensureCfg.WriteNlmEnv = false

	creds, err := auth.EnsureCredentials(ensureCfg)
	if err != nil {
		return err
	}

	client := api.New(creds.AuthToken, creds.Cookies, batchexecute.WithTimeout(timeout))
	_, err = client.ListRecentlyViewedProjects()
	return err
}

func removeOne(in []string, remove string) []string {
	out := make([]string, 0, len(in)-1)
	for _, v := range in {
		if strings.EqualFold(v, remove) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func cookieNames(cookies string) []string {
	cookies = strings.TrimSpace(cookies)
	if cookies == "" {
		return nil
	}

	parts := strings.Split(cookies, ";")
	names := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	return names
}

func stashEnv(keys ...string) func() {
	type entry struct {
		key   string
		value string
		set   bool
	}
	saved := make([]entry, 0, len(keys))
	for _, k := range keys {
		v, ok := os.LookupEnv(k)
		saved = append(saved, entry{key: k, value: v, set: ok})
	}
	return func() {
		for _, e := range saved {
			if e.set {
				_ = os.Setenv(e.key, e.value)
			} else {
				_ = os.Unsetenv(e.key)
			}
		}
	}
}

func updateEnvFileKey(path string, key string, value string) error {
	origBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(origBytes), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		k, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if !strings.EqualFold(k, key) {
			continue
		}
		lines[i] = fmt.Sprintf("%s=%s", key, value)
		found = true
		break
	}

	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
	}

	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out), 0600)
}
