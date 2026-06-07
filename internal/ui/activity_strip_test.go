package ui

import (
	"strings"
	"testing"
	"time"
)

// TestRenderActivityStrip_EmptyWhenNoTools pins the
// behaviour: when the session has no recent tool
// calls, the strip returns an empty string so the
// caller can skip drawing entirely (the chat
// viewport reclaims the row).
func TestRenderActivityStrip_EmptyWhenNoTools(t *testing.T) {
	s := &SessionState{}
	got := renderActivityStrip(s, 120, 0)
	if got != "" {
		t.Errorf("expected empty strip for no recent tools, got %q", got)
	}
}

// TestRenderActivityStrip_ShowsRunningTool pins the
// "sub-agents run by the main ai agent should appear
// at the bottom like claude code" requirement: an
// in-flight tool must show with a spinner + name +
// summary + elapsed timer.
func TestRenderActivityStrip_ShowsRunningTool(t *testing.T) {
	s := &SessionState{
		RecentTools: []RecentToolEntry{
			{
				ToolID:    "t1",
				Name:      "read_file",
				Summary:   "internal/ui/model.go",
				StartedAt: time.Now().Add(-2100 * time.Millisecond),
				Status:    RecentToolPending,
			},
		},
	}
	got := renderActivityStrip(s, 120, 0)
	for _, want := range []string{"read_file", "internal/ui/model.go"} {
		if !strings.Contains(got, want) {
			t.Errorf("strip missing %q, got %q", want, got)
		}
	}
	// Spinner (one of the braille frames) should
	// be present. Frame 0 is "⠋" (U+280B).
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸"}
	found := false
	for _, c := range spinnerChars {
		if strings.Contains(got, c) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a braille spinner in running tool entry, got %q", got)
	}
	// Elapsed time: the running tool started
	// 2.1s ago, so the strip should show
	// something like "2.1s".
	if !strings.Contains(got, "2.1s") {
		t.Errorf("expected '2.1s' elapsed in strip, got %q", got)
	}
}

// TestRenderActivityStrip_DoneAndFailed pins the
// "done" / "failed" suffixes — the user can tell at
// a glance which tools finished vs errored.
func TestRenderActivityStrip_DoneAndFailed(t *testing.T) {
	s := &SessionState{
		RecentTools: []RecentToolEntry{
			{
				ToolID:    "t1",
				Name:      "edit_file",
				Summary:   "foo.go",
				StartedAt: time.Now().Add(-300 * time.Millisecond),
				EndedAt:   time.Now().Add(-200 * time.Millisecond),
				Status:    RecentToolDone,
			},
			{
				ToolID:    "t2",
				Name:      "run_shell",
				Summary:   "npm test",
				StartedAt: time.Now().Add(-1 * time.Second),
				EndedAt:   time.Now().Add(-500 * time.Millisecond),
				Status:    RecentToolFailed,
				IsError:   true,
			},
		},
	}
	got := renderActivityStrip(s, 120, 0)
	if !strings.Contains(got, "(done)") {
		t.Errorf("expected '(done)' marker on completed tool, got %q", got)
	}
	if !strings.Contains(got, "(failed)") {
		t.Errorf("expected '(failed)' marker on failed tool, got %q", got)
	}
}

// TestRenderActivityStrip_PushDropsOldest pins the
// FIFO behaviour: once we hit recentToolsMax (5),
// pushing a new entry drops the oldest. This is
// what keeps the strip from overflowing the single
// row we have for it.
func TestRenderActivityStrip_PushDropsOldest(t *testing.T) {
	s := &SessionState{}
	for i := 0; i < recentToolsMax+3; i++ {
		s.pushRecentTool(RecentToolEntry{
			ToolID: string(rune('a' + i)),
			Name:   "tool",
		})
	}
	if len(s.RecentTools) != recentToolsMax {
		t.Errorf("expected buffer to cap at %d, got %d", recentToolsMax, len(s.RecentTools))
	}
	// The oldest 3 ('a', 'b', 'c') should have
	// been dropped; the newest 3 should still
	// be there.
	for i := 0; i < 3; i++ {
		want := string(rune('a' + i + 3)) // 'd', 'e', 'f'
		if s.RecentTools[i].ToolID != want {
			t.Errorf("entry %d: expected %q, got %q", i, want, s.RecentTools[i].ToolID)
		}
	}
}

// TestRenderActivityStrip_MarkDoneFindsMatchingEntry
// pins the lookup: a tool_result event arrives with
// a toolID; we look it up in the recent-tools
// buffer and flip its status to done (or failed).
func TestRenderActivityStrip_MarkDoneFindsMatchingEntry(t *testing.T) {
	s := &SessionState{
		RecentTools: []RecentToolEntry{
			{ToolID: "t1", Name: "read_file", Status: RecentToolPending},
			{ToolID: "t2", Name: "edit_file", Status: RecentToolPending},
		},
	}
	s.markRecentToolDone("t1", false)
	if s.RecentTools[0].Status != RecentToolDone {
		t.Errorf("expected t1 marked done, got %v", s.RecentTools[0].Status)
	}
	if s.RecentTools[1].Status != RecentToolPending {
		t.Errorf("expected t2 still pending, got %v", s.RecentTools[1].Status)
	}

	s.markRecentToolDone("t2", true)
	if s.RecentTools[1].Status != RecentToolFailed {
		t.Errorf("expected t2 marked failed, got %v", s.RecentTools[1].Status)
	}
}
