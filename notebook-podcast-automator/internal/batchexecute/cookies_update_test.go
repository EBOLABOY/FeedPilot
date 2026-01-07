package batchexecute

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClient_MergesSetCookieIntoNextRequest(t *testing.T) {
	t.Setenv("NLM_COOKIE_ALLOWLIST", "")
	t.Setenv("NLM_COOKIES", "a=old")

	var mu sync.Mutex
	var cookiesSeen []string
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cookiesSeen = append(cookiesSeen, r.Header.Get("Cookie"))
		requestCount++
		mu.Unlock()

		w.Header().Set("Set-Cookie", "a=new; Path=/; HttpOnly")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, ")]}'\n[[\"wrb.fr\",\"test\",\"[]\",null,null,null,\"generic\"]]")
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	httpClient := server.Client()
	httpClient.Timeout = 5 * time.Second

	client := NewClient(Config{
		Host:      host,
		App:       "test",
		AuthToken: "test-token",
		Cookies:   "a=old",
		UseHTTP:   true,
	}, WithHTTPClient(httpClient))

	if _, err := client.Do(RPC{ID: "test", Args: []interface{}{}}); err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if _, err := client.Do(RPC{ID: "test", Args: []interface{}{}}); err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
	if len(cookiesSeen) != 2 {
		t.Fatalf("expected 2 cookie captures, got %d", len(cookiesSeen))
	}
	if !strings.Contains(cookiesSeen[0], "a=old") {
		t.Fatalf("first request Cookie=%q, expected to contain a=old", cookiesSeen[0])
	}
	if !strings.Contains(cookiesSeen[1], "a=new") {
		t.Fatalf("second request Cookie=%q, expected to contain a=new", cookiesSeen[1])
	}

	if got := strings.TrimSpace(os.Getenv("NLM_COOKIES")); !strings.Contains(got, "a=new") {
		t.Fatalf("expected NLM_COOKIES to be updated, got %q", got)
	}
}
