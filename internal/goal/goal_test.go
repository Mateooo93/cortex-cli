package goal

import (
	"testing"
)

func TestParseEvaluatorResponse_Yes(t *testing.T) {
	tests := []string{
		"VERDICT: YES\nREASON: All tests pass and lint is clean.",
		"VERDICT:YES\nREASON: The build succeeded with exit code 0.",
		"verdict: yes\nreason: goal achieved.",
		"YES\nReason: done.",
	}
	for _, tc := range tests {
		result := parseEvaluatorResponse(tc)
		if !result.Met {
			t.Errorf("expected Met=true, got false for input: %q", tc)
		}
		if result.Reason == "" {
			t.Errorf("expected non-empty reason for input: %q", tc)
		}
	}
}

func TestParseEvaluatorResponse_No(t *testing.T) {
	tests := []string{
		"VERDICT: NO\nREASON: Two tests still failing in test/auth.",
		"VERDICT:NO\nREASON: Build not yet attempted.",
		"verdict: no\nreason: work in progress.",
	}
	for _, tc := range tests {
		result := parseEvaluatorResponse(tc)
		if result.Met {
			t.Errorf("expected Met=false, got true for input: %q", tc)
		}
		if result.Reason == "" {
			t.Errorf("expected non-empty reason for input: %q", tc)
		}
	}
}

func TestParseEvaluatorResponse_Malformed(t *testing.T) {
	// Should handle missing fields gracefully
	result := parseEvaluatorResponse("unexpected response without verdict field")
	// Should default to Met=false
	if result.Met {
		t.Errorf("expected Met=false for malformed input")
	}
	// Should capture the response as reason
	if result.Reason == "" {
		t.Errorf("expected non-empty reason for malformed input")
	}
}

func TestTruncateTranscript(t *testing.T) {
	short := "short transcript"
	result := truncateTranscript(short, 100)
	if result != short {
		t.Errorf("short transcript was modified: got %q", result)
	}

	long := make([]byte, 10000)
	for i := range long {
		long[i] = 'x'
	}
	result = truncateTranscript(string(long), 100)
	if len(result) > 200 {
		t.Errorf("truncated transcript too long: %d chars", len(result))
	}
}

func TestBuildEvaluatorMessages(t *testing.T) {
	msgs := buildEvaluatorMessages("tests pass", "transcript content")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected system role, got %s", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("expected user role, got %s", msgs[1].Role)
	}
}

func TestManager_Set_Clear(t *testing.T) {
	m := &Manager{}
	if m.Active() != nil {
		t.Error("expected nil goal in new manager")
	}

	g := m.Set("all tests pass")
	if g == nil || g.Status != StatusActive {
		t.Fatal("expected active goal after Set")
	}

	active := m.Active()
	if active == nil || active.Condition != "all tests pass" {
		t.Error("Active() returned wrong goal")
	}

	m.Clear()
	if m.Active() != nil {
		t.Error("expected nil active goal after Clear")
	}

	state := m.State()
	if state == nil || state.Status != StatusCleared {
		t.Error("expected cleared status in State()")
	}
}

func TestManager_Set_Replaces(t *testing.T) {
	m := &Manager{}
	m.Set("first goal")
	m.Set("second goal")
	state := m.State()
	// Note: Set replaces, so only one goal exists
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	// The first goal was replaced; record not preserved
	if state.Condition != "second goal" {
		t.Errorf("expected second goal, got %q", state.Condition)
	}
}

func TestStatus_Constants(t *testing.T) {
	// Verify status constants are distinct
	statuses := []Status{StatusInactive, StatusActive, StatusAchieved, StatusCleared, StatusFailed}
	seen := map[Status]bool{}
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate status: %s", s)
		}
		seen[s] = true
	}
}
