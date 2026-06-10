package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseSkillFile(t *testing.T) {
	meta, body, err := parseSkillFile([]byte(`---
name: release-checklist
description: Use when preparing a release.
enabled: true
---

# Release checklist
`))
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "release-checklist" || !meta.Enabled {
		t.Fatalf("meta = %#v", meta)
	}
	if body == "" {
		t.Fatal("expected body")
	}
}

func TestListMcpServersEndpoint(t *testing.T) {
	srv := &server{startedAt: time.Now()}
	mux := http.NewServeMux()
	registerExtensionRoutes(mux, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/mcp/servers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
