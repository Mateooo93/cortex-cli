package ui

import "strings"

// detectWorkflowIntent returns true when the user's message
// hints that a multi-agent workflow would be useful. The
// heuristic is intentionally conservative — we don't want to
// fire a workflow for every "I need a function" message.
//
// Trigger phrases (any one of them):
//   - "workflow", "swarm"
//   - "multiple agents", "in parallel", "run agents"
//   - "team of agents", "coordinator", "orchestrator"
//   - "rufflow", "ruflo"  (typos of the ruflo project)
//   - multi-component project signals (landing page with
//     multiple sections, full app build, refactor across many
//     files, etc.) — these often benefit from a workflow
//     because they combine planning + implementation +
//     review + testing steps.
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
	// Multi-component project signals. These fire when the
	// user's message names 3+ distinct deliverables or refers
	// to a substantial build (landing page with several
	// sections, full app, refactor across many files, etc.).
	// The user reported: "make me a landing page for an app
	// called cadence with pricing sections hero and mobile
	// responsiveness" — this is a multi-component task that
	// would benefit from a workflow but the old detector
	// missed it because no trigger phrase was present.
	multiComponentSignals := []string{
		"landing page",
		"homepage",
		"hero section",
		"pricing section",
		"pricing page",
		"full app",
		"full site",
		"full build",
		"end to end",
		"e2e app",
		"build me a",
		"build a full",
		"build the whole",
		"create a full",
		"create me a",
		"make me a",
		"make a full",
		"make a complete",
		"with sections",
		"with multiple sections",
		"with hero",
		"with pricing",
		"with mobile",
		"mobile responsive",
		"responsive design",
	}
	hits := 0
	for _, t := range multiComponentSignals {
		if strings.Contains(lower, t) {
			hits++
		}
	}
	// Two or more signals = strong workflow candidate.
	// (E.g. "landing page" + "pricing" + "hero" + "mobile
	// responsive" would hit 4.)
	if hits >= 2 {
		return true
	}
	// Or a single very strong signal like "build me a" +
	// "landing page" / "full app" / "full site".
	strongSignals := []string{
		"build me a landing page",
		"build a landing page",
		"build me a full app",
		"build a full app",
		"build me a full site",
		"build a full site",
		"make me a landing page",
		"make a landing page",
		"make me a full app",
		"make a full app",
		"make me a full site",
		"make a full site",
		"create me a full app",
		"create a full app",
		"create me a landing page",
		"create a landing page",
		"make me a complete",
	}
	for _, t := range strongSignals {
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

// isSubstantivePrompt returns true when the user's message looks
// like a multi-step engineering task that would benefit from
// workflow orchestration — even without explicit trigger keywords.
// Used by ultracode mode to auto-dispatch workflows for every
// substantive request.
func isSubstantivePrompt(lower string) bool {
	// Multi-component signals (landing page, full app, etc.)
	multiComponent := []string{
		"landing page", "homepage", "hero section", "pricing section",
		"full app", "full site", "full build", "end to end", "e2e",
		"build me a", "build a full", "create a full", "make me a",
		"make a full", "make a complete", "with sections",
		"with multiple sections", "with hero", "with pricing",
		"mobile responsive", "responsive design",
	}
	for _, t := range multiComponent {
		if strings.Contains(lower, t) {
			return true
		}
	}

	// Multi-file refactors
	multiFile := []string{
		"refactor", "migrate", "across the codebase", "every file",
		"all files", "every endpoint", "across all", "bulk update",
		"batch update", "across the repo",
	}
	for _, t := range multiFile {
		if strings.Contains(lower, t) {
			return true
		}
	}

	// Audit / review tasks
	auditTerms := []string{
		"audit", "security review", "code review of",
		"review every", "scan for", "find all",
	}
	for _, t := range auditTerms {
		if strings.Contains(lower, t) {
			return true
		}
	}

	return false
}
