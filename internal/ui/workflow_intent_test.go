package ui

import "testing"

func TestDetectWorkflowIntent_ExplicitPhrases(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"please make a workflow for this refactor", true},
		{"start workflow to build the API", true},
		{"what is a workflow tab", false},
		{"run agents in parallel on this", true},
	}
	for _, tc := range cases {
		got := detectWorkflowIntent(tc.input)
		if got != tc.want {
			t.Fatalf("detectWorkflowIntent(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestPickWorkflowPreset_KeywordRouting(t *testing.T) {
	if p := pickWorkflowPreset("run the tests for auth"); p.Name != "test" {
		t.Fatalf("got %q, want test", p.Name)
	}
	if p := pickWorkflowPreset("write readme docs"); p.Name != "docs" {
		t.Fatalf("got %q, want docs", p.Name)
	}
	if p := pickWorkflowPreset("implement feature X"); p.Name != "code" {
		t.Fatalf("got %q, want code", p.Name)
	}
}

func TestIsUltracodeWorkflowCandidate_BroadEngineeringTasks(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"fix the failing unit test in auth package", true},
		{"add retry logic to the HTTP client", true},
		{"what is golang", false},
		{"how does this function work", false},
		{"refactor the session layer for clarity", true},
		{"ok", false},
	}
	for _, tc := range cases {
		got := isUltracodeWorkflowCandidate(tc.input)
		if got != tc.want {
			t.Fatalf("isUltracodeWorkflowCandidate(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}