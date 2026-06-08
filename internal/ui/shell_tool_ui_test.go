package ui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderShellToolCallDisplay_PendingShowsSpinnerAndTimer(t *testing.T) {
	s := NewStyles(true)
	started := time.Now().Add(-2500 * time.Millisecond)
	rendered := renderShellToolCallDisplay("run_shell", "$ go test ./...", "", [4]string{}, ToolRunPending, started, time.Time{}, 0, s)
	for _, want := range []string{"shell", "$ go test", "running", "2.5s"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("missing %q in %q", want, rendered)
		}
	}
	foundSpinner := false
	for _, c := range activityStripSpinnerFrames {
		if strings.Contains(rendered, c) {
			foundSpinner = true
			break
		}
	}
	if !foundSpinner {
		t.Fatalf("expected spinner in pending shell render, got %q", rendered)
	}
}

func TestRenderShellToolCallDisplay_DoneShowsSucceeded(t *testing.T) {
	s := NewStyles(true)
	started := time.Now().Add(-3 * time.Second)
	ended := time.Now()
	rendered := renderShellToolCallDisplay("run_shell", "$ make build", "", [4]string{}, ToolRunDone, started, ended, 0, s)
	if !strings.Contains(rendered, "✓") {
		t.Fatalf("expected success marker, got %q", rendered)
	}
	if !strings.Contains(rendered, "succeeded") {
		t.Fatalf("expected succeeded label, got %q", rendered)
	}
}

func TestRenderShellToolCallDisplay_FailedShowsFailed(t *testing.T) {
	s := NewStyles(true)
	started := time.Now().Add(-1500 * time.Millisecond)
	ended := time.Now()
	rendered := renderShellToolCallDisplay("bash", "$ npm test", "", [4]string{}, ToolRunFailed, started, ended, 0, s)
	if !strings.Contains(rendered, "✗") {
		t.Fatalf("expected failure marker, got %q", rendered)
	}
	if !strings.Contains(rendered, "failed") {
		t.Fatalf("expected failed label, got %q", rendered)
	}
}