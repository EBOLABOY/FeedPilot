package batchexecute

import (
	"fmt"
	"net/http"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// NewFingerprintedClient creates a new http.Client that simulates a real browser's TLS fingerprint (JA3/JA4)
// This is critical for bypassing Google's anti-bot protections.
func NewFingerprintedClient(timeout time.Duration, proxyURL string) (*http.Client, error) {
	jar := tls_client.NewCookieJar()

	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(int(timeout.Seconds())),
		tls_client.WithClientProfile(profiles.Chrome_131), // Upgraded to Chrome 131 for better JA4 accuracy
		tls_client.WithNotFollowRedirects(),
		tls_client.WithCookieJar(jar), // Use the custom cookie jar
	}

	if proxyURL != "" {
		options = append(options, tls_client.WithProxyUrl(proxyURL))
	}

	// Create the TLS client
	c, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tls client: %w", err)
	}

	return &http.Client{
		Transport: &TLSClientRoundTripper{internal: c},
		Timeout:   timeout,
		Jar:       nil, // Cookies are handled by the internal client
	}, nil
}

// TLSClientRoundTripper adapts the bogdanfinn/tls-client to standard http.RoundTripper interface
type TLSClientRoundTripper struct {
	internal tls_client.HttpClient
}

func (t *TLSClientRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Convert net/http.Request to fhttp.Request
	fReq, err := fhttp.NewRequest(req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create fhttp request: %w", err)
	}

	// Copy headers
	for k, vv := range req.Header {
		for _, v := range vv {
			fReq.Header.Add(k, v)
		}
	}

	// EXECUTE
	fResp, err := t.internal.Do(fReq)
	if err != nil {
		// Try to map fhttp error to net/http error types if possible, or just wrap
		return nil, err
	}

	// Convert fhttp.Response to net/http.Response
	resp := &http.Response{
		Status:           fResp.Status,
		StatusCode:       fResp.StatusCode,
		Proto:            fResp.Proto,
		ProtoMajor:       fResp.ProtoMajor,
		ProtoMinor:       fResp.ProtoMinor,
		Header:           make(http.Header),
		Body:             fResp.Body, // Body is io.ReadCloser, should be compatible
		ContentLength:    fResp.ContentLength,
		TransferEncoding: fResp.TransferEncoding,
		Close:            fResp.Close,
		Uncompressed:     fResp.Uncompressed,
		Trailer:          make(http.Header),
		Request:          req, // Point back to original request
		TLS:              nil, // We don't easily get TLS state back, usually fine
	}

	// Copy response headers
	for k, vv := range fResp.Header {
		for _, v := range vv {
			resp.Header.Add(k, v)
		}
	}

	// Handle Trailer if needed (often empty)

	return resp, nil
}
