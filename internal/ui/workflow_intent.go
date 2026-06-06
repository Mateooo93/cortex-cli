package ui

import "strings"

// detectWorkflowIntent returns true when the user's message
// hints that a multi-agent workflow would be useful. The
// heuristic is intentionally conservative \u2014 we don't want to
// fire a workflow for every "I need a function" message.
//
// Trigger phrases (any one of them):
//   - "workflow", "swarm"
//   - "multiple agents", "in parallel", "run agents"
//   - "team of agents", "coordinator", "orchestrator"
//   - "rufflow", "ruflo"  (typos of the ruflo project)
//
// Plus the user can always invoke /workflow explicitly.
func detectWorkflowIntent(lower string) bool {
	// Bare "workflow" mentions are too noisy (e.g. "what
	// is the workflow tab for?"); require a verb prefix
	// to fire.
	workflowVerbs := []string{
		"run a workflow",
		"run workflow",
		"use a workflow",
		"use workflow",
		"start a workflow",
		"start workflow",
		"as a workflow",
		"dispatch a workflow",
		"via a workflow",
		"with a workflow",
		"make a workflow",
		"create a workflow",
		"new workflow",
	}
	for _, t := range workflowVerbs {
		if strings.Contains(lower, t) {
			return true
		}
	}
	// These phrases are unambiguous so we don't gate them.
	unambiguous := []string{
		"swarm",
		"multiple agents",
		"in parallel",
		"run agents",
		"team of agents",
		"coordinator",
		"orchestrator",
		"ruflo",
		"rufflow",
		"dispatch agents",
		"sub-agents",
		"subagents",
	}
	for _, t := range unambiguous {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// pickWorkflowPreset returns the best preset for a given
// user message. The heuristic is a simple keyword match; the
// user can always pick a different preset from the Workflows
// tab if the default isn't right.
func pickWorkflowPreset(input string) struct {
	Name        string
	Strategy    string
	MaxAgents   int
	Description string
} {
	// Note: this returns an anonymous struct because
	// importing workflow.Preset creates a cycle (the UI
	// package is already imported by internal/workflow in
	// the agent definitions). The fields line up with
	// workflow.BuiltinPresets; the engine accepts the same
	// shape via its Start() parameters.
	type presetShape struct {
		Name        string
		Strategy    string
		MaxAgents   int
		Description string
	}
	// Be defensive about case — the function used to
	// expect the caller to lowercase first, but that's
	// easy to forget and a silent miss on a preset
	// selection is bad UX.
	lower := strings.ToLower(input)
	switch {
	case strings.Contains(lower, "test"),
		strings.Contains(lower, "ci "),
		strings.Contains(lower, "run the tests"):
		return presetShape{Name: "test", Strategy: "testing", MaxAgents: 4, Description: "Write and run tests for an existing code change."}
	case strings.Contains(lower, "review"),
		strings.Contains(lower, "critique"),
		strings.Contains(lower, "audit"):
		return presetShape{Name: "review", Strategy: "optimization", MaxAgents: 4, Description: "Review a diff or plan, surface issues, and suggest fixes."}
	case strings.Contains(lower, "docs"),
		strings.Contains(lower, "readme"),
		strings.Contains(lower, "documentation"):
		return presetShape{Name: "docs", Strategy: "research", MaxAgents: 3, Description: "Write or improve project documentation."}
	case strings.Contains(lower, "research"),
		strings.Contains(lower, "investigate"),
		strings.Contains(lower, "look up"),
		strings.Contains(lower, "explore the"):
		return presetShape{Name: "research", Strategy: "research", MaxAgents: 3, Description: "Gather documentation and reference material, then summarise findings."}
	default:
		return presetShape{Name: "code", Strategy: "development", MaxAgents: 5, Description: "Plan, implement, review, and test a coding task end-to-end."}
	}
}
