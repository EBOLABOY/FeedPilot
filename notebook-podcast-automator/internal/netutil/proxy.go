package netutil

import (
	"net"
	"os"
	"strings"
	"time"
)

func MaybeSetLocalProxy() {
	if os.Getenv("HTTP_PROXY") != "" || os.Getenv("HTTPS_PROXY") != "" {
		return
	}

	if proxyURL := strings.TrimSpace(os.Getenv("NLM_PROXY_URL")); proxyURL != "" {
		_ = os.Setenv("HTTP_PROXY", proxyURL)
		_ = os.Setenv("HTTPS_PROXY", proxyURL)
		return
	}

	conn, err := net.DialTimeout("tcp", "127.0.0.1:10809", 200*time.Millisecond)
	if err != nil {
		return
	}
	_ = conn.Close()

	_ = os.Setenv("HTTP_PROXY", "http://127.0.0.1:10809")
	_ = os.Setenv("HTTPS_PROXY", "http://127.0.0.1:10809")
}
