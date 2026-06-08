package tools

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	webFetchDefaultMaxBytes = 512 * 1024
	webFetchMaxRedirects    = 5
	webFetchTimeout         = 30 * time.Second
	webFetchUserAgent       = "cortex-cli/1.0 (+https://github.com/Mateooo93/cortex-cli)"
)

// WebFetchTool downloads a public HTTP(S) URL and returns the response body
// as text for the model to read.
type WebFetchTool struct {
	// hostAllowed overrides the default private-host block (tests only).
	hostAllowed func(host string) error
}

func (t *WebFetchTool) checkHost(host string) error {
	if t.hostAllowed != nil {
		return t.hostAllowed(host)
	}
	return rejectPrivateFetchHost(host)
}

func (t *WebFetchTool) Name() string { return "web_fetch" }
func (t *WebFetchTool) Description() string {
	return "Fetch a public HTTP or HTTPS URL and return the response body as text. " +
		"Use for documentation, API responses, and articles. Does not execute JavaScript."
}
func (t *WebFetchTool) Parameters() map[string]Param {
	return map[string]Param{
		"url":      {Type: "string", Description: "HTTP or HTTPS URL to fetch", Required: true},
		"maxBytes": {Type: "number", Description: "Maximum response bytes to return (default 524288)"},
	}
}

func (t *WebFetchTool) Run(ctx Context, args map[string]any) (Result, error) {
	rawURL, _ := args["url"].(string)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return Result{OK: false, Error: "url is required"}, nil
	}
	maxBytes := webFetchDefaultMaxBytes
	if v, ok := args["maxBytes"]; ok {
		switch n := v.(type) {
		case float64:
			if n > 0 {
				maxBytes = int(n)
			}
		case int:
			if n > 0 {
				maxBytes = n
			}
		case int64:
			if n > 0 {
				maxBytes = int(n)
			}
		}
	}
	if maxBytes > 2*1024*1024 {
		maxBytes = 2 * 1024 * 1024
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return Result{OK: false, Error: "invalid url: " + err.Error()}, nil
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Result{OK: false, Error: "only http and https URLs are supported"}, nil
	}
	if parsed.Host == "" {
		return Result{OK: false, Error: "url must include a host"}, nil
	}
	if err := t.checkHost(parsed.Hostname()); err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}

	start := time.Now()
	client := &http.Client{
		Timeout: webFetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= webFetchMaxRedirects {
				return fmt.Errorf("stopped after %d redirects", webFetchMaxRedirects)
			}
			if err := t.checkHost(req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	req.Header.Set("User-Agent", webFetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return Result{OK: false, Error: err.Error()}, nil
	}
	truncated := false
	if len(body) > maxBytes {
		body = body[:maxBytes]
		truncated = true
	}

	elapsed := time.Since(start).Milliseconds()
	contentType := resp.Header.Get("Content-Type")
	if i := strings.Index(contentType, ";"); i >= 0 {
		contentType = strings.TrimSpace(contentType[:i])
	}
	if contentType == "" {
		contentType = "unknown"
	}

	var out strings.Builder
	fmt.Fprintf(&out, "URL: %s\n", resp.Request.URL.String())
	fmt.Fprintf(&out, "Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
	fmt.Fprintf(&out, "Content-Type: %s\n", contentType)
	fmt.Fprintf(&out, "Bytes: %d", len(body))
	if truncated {
		out.WriteString(" (truncated)")
	}
	out.WriteString("\n\n")
	out.Write(body)

	return Result{
		OK:     true,
		Output: out.String(),
		Details: map[string]any{
			"elapsed_ms":   elapsed,
			"url":          resp.Request.URL.String(),
			"status_code":  resp.StatusCode,
			"content_type": contentType,
			"bytes":        len(body),
			"truncated":    truncated,
		},
	}, nil
}

// rejectPrivateFetchHost blocks loopback, link-local, and RFC1918 targets to
// reduce SSRF risk when fetching user-supplied URLs.
func rejectPrivateFetchHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("url host is empty")
	}
	if host == "localhost" {
		return fmt.Errorf("fetching localhost is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("fetching private or loopback addresses is not allowed")
		}
		return nil
	}
	return nil
}