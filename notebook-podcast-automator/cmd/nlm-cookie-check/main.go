package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
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

var snlm0eRe = regexp.MustCompile(`"SNlM0e"\s*:\s*"([^"]+)"`)

type checkResult struct {
	label              string
	allowlist          string
	cookieNames        []string
	homeStatus         int
	homeLocation       string
	homeHasSNlM0e      bool
	homeErr            string
	homeSetCookies     []string
	homeFPStatus       int
	homeFPLocation     string
	homeFPHasSNlM0e    bool
	homeFPErr          string
	homeFPSetCookies   []string
	authTokenLen       int
	authTokenHasColon  bool
	authTokenExpiry    string
	authTokenExpired   bool
	authTokenRefreshed bool
	projectCount       int
	projectSummaries   []string
	err                error
}

func main() {
	var dotenvPath string
	var timeoutSeconds int
	var forceRefresh bool
	var printProjects bool
	var maxProjects int
	var noProxy bool
	var writeDotenv bool
	var persistCookies bool
	flag.StringVar(&dotenvPath, "dotenv", ".env", "path to .env file (empty to skip)")
	flag.IntVar(&timeoutSeconds, "timeout", 60, "HTTP timeout in seconds")
	flag.BoolVar(&forceRefresh, "force-refresh", false, "force refresh SNlM0e token from web")
	flag.BoolVar(&printProjects, "print-projects", false, "print notebook/project titles and IDs")
	flag.IntVar(&maxProjects, "max-projects", 20, "max projects to print when -print-projects")
	flag.BoolVar(&noProxy, "no-proxy", false, "disable HTTP proxy for all checks")
	flag.BoolVar(&writeDotenv, "write-dotenv", false, "write refreshed token/cookies back to .env and ~/.nlm/env")
	flag.BoolVar(&persistCookies, "persist-cookies", false, "persist NLM_COOKIES when refreshed (implies -write-dotenv)")
	flag.Parse()

	if strings.TrimSpace(dotenvPath) != "" {
		_ = godotenv.Overload(dotenvPath)
	}
	netutil.MaybeSetLocalProxy()

	allowlist := strings.TrimSpace(os.Getenv("NLM_COOKIE_ALLOWLIST"))

	results := []checkResult{
		runCheck("baseline", "", forceRefresh, printProjects, maxProjects, noProxy, writeDotenv, persistCookies, time.Duration(timeoutSeconds)*time.Second),
	}
	if allowlist != "" {
		results = append(results, runCheck("allowlist", allowlist, forceRefresh, printProjects, maxProjects, noProxy, writeDotenv, persistCookies, time.Duration(timeoutSeconds)*time.Second))
	}

	ok := true
	for _, res := range results {
		if res.err != nil {
			ok = false
		}
		printResult(res)
	}

	if !ok {
		os.Exit(1)
	}
}

func runCheck(label string, allowlist string, forceRefresh bool, printProjects bool, maxProjects int, noProxy bool, writeDotenv bool, persistCookies bool, timeout time.Duration) checkResult {
	restore := stashEnv("NLM_COOKIE_ALLOWLIST", "NLM_REFRESH_TOKEN", "NLM_PERSIST_COOKIES")
	defer restore()

	tokenBefore := strings.TrimSpace(os.Getenv("NLM_AUTH_TOKEN"))

	if strings.TrimSpace(allowlist) == "" {
		_ = os.Unsetenv("NLM_COOKIE_ALLOWLIST")
	} else {
		_ = os.Setenv("NLM_COOKIE_ALLOWLIST", allowlist)
	}
	if forceRefresh {
		_ = os.Setenv("NLM_REFRESH_TOKEN", "true")
	}
	if persistCookies {
		writeDotenv = true
		_ = os.Setenv("NLM_PERSIST_COOKIES", "true")
	}

	ensureCfg := auth.DefaultEnsureConfig()
	ensureCfg.WriteDotenv = writeDotenv
	ensureCfg.WriteNlmEnv = writeDotenv

	creds, err := auth.EnsureCredentials(ensureCfg)
	if err != nil {
		return checkResult{label: label, allowlist: allowlist, err: err}
	}

	filteredCookies, _ := cookieutil.FilterCookieHeader(creds.Cookies, strings.TrimSpace(os.Getenv("NLM_COOKIE_ALLOWLIST")))
	names := cookieNames(filteredCookies)
	sort.Strings(names)

	homeStatus, homeLocation, homeHasSNlM0e, homeSetCookies, homeErr := probeHomepage(filteredCookies, timeout, noProxy)
	homeErrStr := ""
	if homeErr != nil {
		homeErrStr = homeErr.Error()
	}

	homeFPStatus, homeFPLocation, homeFPHasSNlM0e, homeFPSetCookies, homeFPErr := probeHomepageFingerprint(filteredCookies, timeout)
	homeFPErrStr := ""
	if homeFPErr != nil {
		homeFPErrStr = homeFPErr.Error()
	}

	_, expiry, expiryErr := auth.ParseAuthToken(creds.AuthToken)
	expiryStr := ""
	expired := false
	if expiryErr == nil {
		expiryStr = expiry.UTC().Format(time.RFC3339)
		expired = time.Until(expiry) <= 0
	}

	httpClient := newHTTPClient(timeout, noProxy)
	var opts []batchexecute.Option
	if noProxy {
		opts = append(opts, batchexecute.WithHTTPClient(httpClient))
	} else {
		opts = append(opts, batchexecute.WithTimeout(timeout))
	}
	client := api.New(creds.AuthToken, creds.Cookies, opts...)

	projects, err := client.ListRecentlyViewedProjects()
	if err != nil {
		return checkResult{
			label:              label,
			allowlist:          allowlist,
			cookieNames:        names,
			homeStatus:         homeStatus,
			homeLocation:       homeLocation,
			homeHasSNlM0e:      homeHasSNlM0e,
			homeErr:            homeErrStr,
			homeSetCookies:     homeSetCookies,
			homeFPStatus:       homeFPStatus,
			homeFPLocation:     homeFPLocation,
			homeFPHasSNlM0e:    homeFPHasSNlM0e,
			homeFPErr:          homeFPErrStr,
			homeFPSetCookies:   homeFPSetCookies,
			authTokenLen:       len(creds.AuthToken),
			authTokenHasColon:  strings.Contains(creds.AuthToken, ":"),
			authTokenExpiry:    expiryStr,
			authTokenExpired:   expired,
			authTokenRefreshed: tokenBefore != "" && creds.AuthToken != tokenBefore,
			err:                err,
		}
	}

	var summaries []string
	if printProjects && len(projects) > 0 {
		limit := maxProjects
		if limit <= 0 || limit > len(projects) {
			limit = len(projects)
		}
		summaries = make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			p := projects[i]
			if p == nil {
				continue
			}
			title := strings.TrimSpace(p.Title)
			if title == "" {
				title = "(untitled)"
			}
			summaries = append(summaries, fmt.Sprintf("%d. %s (%s)", i+1, title, strings.TrimSpace(p.ProjectId)))
		}
	}

	return checkResult{
		label:              label,
		allowlist:          allowlist,
		cookieNames:        names,
		homeStatus:         homeStatus,
		homeLocation:       homeLocation,
		homeHasSNlM0e:      homeHasSNlM0e,
		homeErr:            homeErrStr,
		homeSetCookies:     homeSetCookies,
		homeFPStatus:       homeFPStatus,
		homeFPLocation:     homeFPLocation,
		homeFPHasSNlM0e:    homeFPHasSNlM0e,
		homeFPErr:          homeFPErrStr,
		homeFPSetCookies:   homeFPSetCookies,
		authTokenLen:       len(creds.AuthToken),
		authTokenHasColon:  strings.Contains(creds.AuthToken, ":"),
		authTokenExpiry:    expiryStr,
		authTokenExpired:   expired,
		authTokenRefreshed: tokenBefore != "" && creds.AuthToken != tokenBefore,
		projectCount:       len(projects),
		projectSummaries:   summaries,
	}
}

func printResult(res checkResult) {
	al := "(disabled)"
	if strings.TrimSpace(res.allowlist) != "" {
		al = res.allowlist
	}

	if res.err != nil {
		fmt.Printf("[%s] FAIL allowlist=%s cookies_sent=%d home=%d snlm0e=%v home_fp=%d snlm0e_fp=%v token_len=%d token_colon=%v token_expiry=%s token_expired=%v token_refreshed=%v err=%v\n",
			res.label, al, len(res.cookieNames), res.homeStatus, res.homeHasSNlM0e, res.homeFPStatus, res.homeFPHasSNlM0e, res.authTokenLen, res.authTokenHasColon, res.authTokenExpiry, res.authTokenExpired, res.authTokenRefreshed, res.err)
		if strings.TrimSpace(res.homeErr) != "" {
			fmt.Printf("[%s] home_err=%s\n", res.label, strings.TrimSpace(res.homeErr))
		}
		if strings.TrimSpace(res.homeLocation) != "" {
			fmt.Printf("[%s] redirect=%s\n", res.label, res.homeLocation)
		}
		if strings.TrimSpace(res.homeFPErr) != "" {
			fmt.Printf("[%s] home_fp_err=%s\n", res.label, strings.TrimSpace(res.homeFPErr))
		}
		if strings.TrimSpace(res.homeFPLocation) != "" {
			fmt.Printf("[%s] redirect_fp=%s\n", res.label, res.homeFPLocation)
		}
		if len(res.homeSetCookies) > 0 {
			fmt.Printf("[%s] set_cookie=%s\n", res.label, strings.Join(res.homeSetCookies, ","))
		}
		if len(res.homeFPSetCookies) > 0 {
			fmt.Printf("[%s] set_cookie_fp=%s\n", res.label, strings.Join(res.homeFPSetCookies, ","))
		}
		if len(res.cookieNames) > 0 {
			fmt.Printf("[%s] cookie_names=%s\n", res.label, strings.Join(res.cookieNames, ","))
		}
		return
	}

	fmt.Printf("[%s] OK allowlist=%s cookies_sent=%d home=%d snlm0e=%v home_fp=%d snlm0e_fp=%v token_len=%d token_colon=%v token_expiry=%s token_expired=%v token_refreshed=%v projects=%d\n",
		res.label, al, len(res.cookieNames), res.homeStatus, res.homeHasSNlM0e, res.homeFPStatus, res.homeFPHasSNlM0e, res.authTokenLen, res.authTokenHasColon, res.authTokenExpiry, res.authTokenExpired, res.authTokenRefreshed, res.projectCount)
	if strings.TrimSpace(res.homeErr) != "" {
		fmt.Printf("[%s] home_err=%s\n", res.label, strings.TrimSpace(res.homeErr))
	}
	if strings.TrimSpace(res.homeLocation) != "" {
		fmt.Printf("[%s] redirect=%s\n", res.label, res.homeLocation)
	}
	if strings.TrimSpace(res.homeFPErr) != "" {
		fmt.Printf("[%s] home_fp_err=%s\n", res.label, strings.TrimSpace(res.homeFPErr))
	}
	if strings.TrimSpace(res.homeFPLocation) != "" {
		fmt.Printf("[%s] redirect_fp=%s\n", res.label, res.homeFPLocation)
	}
	if len(res.homeSetCookies) > 0 {
		fmt.Printf("[%s] set_cookie=%s\n", res.label, strings.Join(res.homeSetCookies, ","))
	}
	if len(res.homeFPSetCookies) > 0 {
		fmt.Printf("[%s] set_cookie_fp=%s\n", res.label, strings.Join(res.homeFPSetCookies, ","))
	}
	if len(res.cookieNames) > 0 {
		fmt.Printf("[%s] cookie_names=%s\n", res.label, strings.Join(res.cookieNames, ","))
	}
	if len(res.projectSummaries) > 0 {
		fmt.Printf("[%s] projects:\n", res.label)
		for _, line := range res.projectSummaries {
			fmt.Printf("  %s\n", line)
		}
	}
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

func parseSetCookieNames(setCookieHeaders []string) []string {
	if len(setCookieHeaders) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(setCookieHeaders))
	names := make([]string, 0, len(setCookieHeaders))
	for _, raw := range setCookieHeaders {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		first, _, _ := strings.Cut(raw, ";")
		name, _, ok := strings.Cut(first, "=")
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
	sort.Strings(names)
	return names
}

func probeHomepage(cookies string, timeout time.Duration, noProxy bool) (status int, location string, hasSNlM0e bool, setCookieNames []string, err error) {
	client := newHTTPClient(timeout, noProxy)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return 0, "", false, nil, err
	}

	if strings.TrimSpace(cookies) != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", false, nil, err
	}
	defer resp.Body.Close()

	setCookieNames = parseSetCookieNames(resp.Header.Values("Set-Cookie"))
	location = strings.TrimSpace(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, location, false, setCookieNames, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return resp.StatusCode, location, false, setCookieNames, err
	}

	m := snlm0eRe.FindSubmatch(body)
	return resp.StatusCode, location, len(m) >= 2, setCookieNames, nil
}

func probeHomepageFingerprint(cookies string, timeout time.Duration) (status int, location string, hasSNlM0e bool, setCookieNames []string, err error) {
	proxyURL := strings.TrimSpace(os.Getenv("NLM_PROXY_URL"))
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(os.Getenv("HTTPS_PROXY"))
	}
	if proxyURL == "" {
		proxyURL = strings.TrimSpace(os.Getenv("HTTP_PROXY"))
	}

	client, err := batchexecute.NewFingerprintedClient(timeout, proxyURL)
	if err != nil {
		return 0, "", false, nil, err
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return 0, "", false, nil, err
	}

	if strings.TrimSpace(cookies) != "" {
		req.Header.Set("Cookie", cookies)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", false, nil, err
	}
	defer resp.Body.Close()

	setCookieNames = parseSetCookieNames(resp.Header.Values("Set-Cookie"))
	location = strings.TrimSpace(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, location, false, setCookieNames, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return resp.StatusCode, location, false, setCookieNames, err
	}

	m := snlm0eRe.FindSubmatch(body)
	return resp.StatusCode, location, len(m) >= 2, setCookieNames, nil
}

func newHTTPClient(timeout time.Duration, noProxy bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if noProxy {
		tr.Proxy = nil
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
	}
}
