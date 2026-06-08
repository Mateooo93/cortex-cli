package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchTool_FetchesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from web"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{hostAllowed: func(string) error { return nil }}
	res, err := tool.Run(Context{}, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.OK {
		t.Fatalf("result not OK: %s", res.Error)
	}
	if !strings.Contains(res.Output, "hello from web") {
		t.Errorf("output = %q, want body text", res.Output)
	}
	if !strings.Contains(res.Output, "Status: 200") {
		t.Errorf("output missing status line: %q", res.Output)
	}
	if res.Details["elapsed_ms"] == nil {
		t.Error("expected elapsed_ms in details")
	}
}

func TestWebFetchTool_TruncatesAtMaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer srv.Close()

	tool := &WebFetchTool{hostAllowed: func(string) error { return nil }}
	res, err := tool.Run(Context{}, map[string]any{
		"url":      srv.URL,
		"maxBytes": float64(5),
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !res.OK {
		t.Fatalf("result not OK: %s", res.Error)
	}
	if !strings.Contains(res.Output, "01234") {
		t.Errorf("output = %q, want truncated body", res.Output)
	}
	if !strings.Contains(res.Output, "truncated") {
		t.Errorf("output should note truncation: %q", res.Output)
	}
}

func TestWebFetchTool_RejectsPrivateHosts(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/",
		"http://localhost/",
		"http://10.0.0.1/",
		"file:///etc/passwd",
	}
	for _, u := range cases {
		res, err := (&WebFetchTool{}).Run(Context{}, map[string]any{"url": u})
		if err != nil {
			t.Fatalf("Run(%q) error: %v", u, err)
		}
		if res.OK {
			t.Errorf("Run(%q) should fail, got output %q", u, res.Output)
		}
	}
}

func TestWebFetchTool_RegisteredInDefaultTools(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.Get("web_fetch"); !ok {
		t.Fatal("web_fetch not registered in default registry")
	}
}