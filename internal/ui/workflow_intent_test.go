package ui

import "testing"

// TestDetectWorkflowIntent covers the trigger phrases for
// auto-dispatching a workflow when the user mentions
// "workflow", "swarm", "in parallel", etc.
func TestDetectWorkflowIntent(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Positive cases (explicit workflow mentions)
		{"can you run this as a workflow?", true},
		{"dispatch a swarm of agents on this", true},
		{"run multiple agents in parallel", true},
		{"use a ruflo-style multi-agent flow", true},
		{"send sub-agents to investigate", true},
		// Positive cases (multi-component project signals —
		// the user's bug report was "make me a landing page
		// for an app called cadence with pricing sections
		// hero and mobile responsiveness" which should
		// auto-dispatch a workflow).
		{"make me a landing page for cadence with pricing sections and mobile responsiveness", true},
		{"build me a full app for managing todos", true},
		{"create a full site with hero, pricing, and about", true},
		{"build a landing page for my startup", true},
		{"make me a complete e-commerce app", true},
		// Negative cases
		{"fix the bug in main.go", false},
		{"add a function that returns the sum", false},
		{"what is the workflow tab for?", false}, // "workflow" in question, not a directive
		{"", false},
	}
	for _, tt := range tests {
		if got := detectWorkflowIntent(tt.input); got != tt.want {
			t.Errorf("detectWorkflowIntent(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestPickWorkflowPreset covers the keyword-based preset
// selection. The "code" preset is the default fallback.
func TestPickWorkflowPreset(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"write tests for the new feature", "test"},
		{"run the tests in CI", "test"},
		{"review my changes for security issues", "review"},
		{"audit the auth flow", "review"},
		{"write a README for the project", "docs"},
		{"improve the documentation", "docs"},
		{"research the API for me", "research"},
		{"investigate the upstream bug", "research"},
		{"add a new endpoint", "code"},
		{"", "code"},
	}
	for _, tt := range tests {
		got := pickWorkflowPreset(tt.input)
		if got.Name != tt.want {
			t.Errorf("pickWorkflowPreset(%q) = %q, want %q", tt.input, got.Name, tt.want)
		}
	}
}

// TestSlashMenuIncludesWorkflow pins the /workflow slash
// command registration. Without this entry the user has to
// know to use F3 + 'n' instead of the more discoverable
// "/workflow" command.
func TestSlashMenuIncludesWorkflow(t *testing.T) {
	var found bool
	for _, cmd := range slashCommands {
		if cmd.Name == "workflow" {
			found = true
			if cmd.Action != "open_workflow_picker" {
				t.Errorf("/workflow action = %q, want 'open_workflow_picker'", cmd.Action)
			}
			if cmd.Description == "" {
				t.Errorf("/workflow has empty description")
			}
		}
	}
	if !found {
		t.Errorf("expected /workflow in slashCommands")
	}
}
