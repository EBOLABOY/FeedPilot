// Package auth handles authentication and credential refresh for NotebookLM (1:1 Port from nlm_upstream)
package auth

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	SignalerAPIURL = "https://signaler-pa.clients6.google.com/punctual/v1/refreshCreds"
	SignalerAPIKey = "AIzaSyC_pzrI0AjEDXDYcg7kkq3uQEjnXV50pBM"
)

type RefreshClient struct {
	cookies    string
	sapisid    string
	httpClient *http.Client
	debug      bool
}

func NewRefreshClient(cookies string) (*RefreshClient, error) {
	sapisid := extractCookieValue(cookies, "SAPISID")
	if sapisid == "" {
		return nil, fmt.Errorf("SAPISID not found in cookies")
	}

	proxyURL, _ := url.Parse("http://127.0.0.1:10809")
	return &RefreshClient{
		cookies: cookies,
		sapisid: sapisid,
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (r *RefreshClient) RefreshCredentials(gsessionID string) error {
	params := url.Values{}
	params.Set("key", SignalerAPIKey)
	if gsessionID != "" {
		params.Set("gsessionid", gsessionID)
	}

	fullURL := SignalerAPIURL + "?" + params.Encode()
	timestamp := time.Now().Unix()
	authHash := r.generateSAPISIDHASH(timestamp)

	requestBody := []string{"tZf5V3ry"}
	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fullURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("SAPISIDHASH %d_%s", timestamp, authHash))
	req.Header.Set("Content-Type", "application/json+protobuf")
	req.Header.Set("Cookie", r.cookies)
	req.Header.Set("Origin", "https://notebooklm.google.com")
	req.Header.Set("Referer", "https://notebooklm.google.com/")
	req.Header.Set("X-Goog-AuthUser", "0")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (r *RefreshClient) generateSAPISIDHASH(timestamp int64) string {
	origin := "https://notebooklm.google.com"
	data := fmt.Sprintf("%d %s %s", timestamp, r.sapisid, origin)
	hash := sha1.New()
	hash.Write([]byte(data))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func extractCookieValue(cookies, name string) string {
	parts := strings.Split(cookies, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, name+"=") {
			return strings.TrimPrefix(part, name+"=")
		}
	}
	return ""
}

// ExtractGSessionID extracts the gsessionid from NotebookLM (1:1 from nlm_upstream)
func (r *RefreshClient) ExtractGSessionID() (string, error) {
	req, err := http.NewRequest("GET", "https://notebooklm.google.com/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", r.cookies)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	pattern := regexp.MustCompile(`"gsessionid"\s*:\s*"([^"]+)"`)
	matches := pattern.FindSubmatch(body)
	if len(matches) > 1 {
		return string(matches[1]), nil
	}

	return "", fmt.Errorf("gsessionid not found in page")
}
