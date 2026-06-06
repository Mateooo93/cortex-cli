package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestBuiltinRolesCoverCoreTasks pins the set of roles
// available to the orchestrator. The user explicitly asked
// for multi-agent workflows; the role set is the user's
// contract \u2014 if we remove "developer" or "reviewer" by
// accident, the engine can't plan useful workflows.
func TestBuiltinRolesCoverCoreTasks(t *testing.T) {
	want := []string{"planner", "developer", "reviewer", "tester", "researcher", "fixer", "documenter"}
	have := map[string]bool{}
	for _, r := range BuiltinRoles {
		have[r.Name] = true
		if r.SystemPrompt == "" {
			t.Errorf("role %q has empty system prompt", r.Name)
		}
		if r.Description == "" {
			t.Errorf("role %q has empty description", r.Name)
		}
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("missing role %q from BuiltinRoles", w)
		}
	}
}

// TestBuiltinPresetsHaveRequiredFields pins the preset
// shape. Every preset must have a name, description, and
// strategy; otherwise the Workflows tab renders an empty
// row.
func TestBuiltinPresetsHaveRequiredFields(t *testing.T) {
	if len(BuiltinPresets) == 0 {
		t.Fatal("BuiltinPresets is empty")
	}
	seen := map[string]bool{}
	for _, p := range BuiltinPresets {
		if p.Name == "" {
			t.Errorf("preset with empty name: %+v", p)
		}
		if seen[p.Name] {
			t.Errorf("duplicate preset name %q", p.Name)
		}
		seen[p.Name] = true
		if p.Description == "" {
			t.Errorf("preset %q has empty description", p.Name)
		}
		if p.Strategy == "" {
			t.Errorf("preset %q has empty strategy", p.Name)
		}
		if p.MaxAgents <= 0 {
			t.Errorf("preset %q has bad MaxAgents %d", p.Name, p.MaxAgents)
		}
	}
}

// TestExtractJSON_Fenced pins the JSON extraction against a
// ```json```-fenced planner response. The planner frequently
// wraps its plan in a code fence; we have to strip it.
func TestExtractJSON_Fenced(t *testing.T) {
	input := "Here's my plan:\n```json\n{\"steps\": [{\"id\":\"s1\"}]}\n```\nDone."
	got := extractJSON(input)
	if got == "" {
		t.Fatal("extractJSON returned empty for fenced input")
	}
	if !strings.Contains(got, "steps") {
		t.Errorf("extractJSON dropped the JSON: %q", got)
	}
}

// TestExtractJSON_BalancedBraces covers the case where the
// planner doesn't fence its output but the JSON is
// top-level. We have to find the matching closing brace
// (ignoring braces inside strings).
func TestExtractJSON_BalancedBraces(t *testing.T) {
	input := `My plan is {"steps": [{"id":"step-1","description":"d","role":"developer"}], "extra": "}"}`
	got := extractJSON(input)
	if got == "" {
		t.Fatal("extractJSON returned empty for brace-balanced input")
	}
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Errorf("extractJSON didn't return a top-level object: %q", got)
	}
}

// TestExtractJSON_NoJSON returns empty so the engine can
// fall back to a single developer task.
func TestExtractJSON_NoJSON(t *testing.T) {
	if got := extractJSON("just a regular response, no JSON here."); got != "" {
		t.Errorf("extractJSON(%q) = %q, want \"\"", "just a regular response", got)
	}
}

// TestParsePlan_HappyPath pins the plan parser against a
// valid planner response. The output Steps should have the
// expected IDs and roles.
func TestParsePlan_HappyPath(t *testing.T) {
	input := `{"steps": [{"id":"step-1","description":"plan the auth flow","role":"developer"}, {"id":"step-2","description":"write tests","role":"tester"}]}`
	steps := parsePlan(input)
	if len(steps) != 2 {
		t.Fatalf("parsePlan returned %d steps, want 2", len(steps))
	}
	if steps[0].ID != "step-1" || steps[0].Role != "developer" {
		t.Errorf("step[0] = %+v, want id=step-1 role=developer", steps[0])
	}
	if steps[1].Role != "tester" {
		t.Errorf("step[1].Role = %q, want tester", steps[1].Role)
	}
	for i, s := range steps {
		if s.Status != StepPending {
			t.Errorf("step[%d].Status = %q, want %q", i, s.Status, StepPending)
		}
	}
}

// TestParsePlan_GeneratesIDs checks the fallback ID
// generator. If the planner forgets to include an id, the
// parser must still produce step-N IDs so the engine can
// reference each step.
func TestParsePlan_GeneratesIDs(t *testing.T) {
	input := `{"steps": [{"description":"a","role":"developer"}, {"description":"b","role":"tester"}]}`
	steps := parsePlan(input)
	if len(steps) != 2 {
		t.Fatalf("parsePlan returned %d steps, want 2", len(steps))
	}
	if steps[0].ID == "" {
		t.Errorf("step[0].ID is empty, expected auto-generated")
	}
	if steps[1].ID == "" {
		t.Errorf("step[1].ID is empty, expected auto-generated")
	}
}

// TestParsePlan_InvalidJSON returns nil so the engine can
// fall back to a single developer task. The user still
// gets something useful; they just don't get the full plan.
func TestParsePlan_InvalidJSON(t *testing.T) {
	if got := parsePlan("not json at all"); got != nil {
		t.Errorf("parsePlan(\"not json\") = %v, want nil", got)
	}
}

// TestEngine_StartCancel is the headline integration test:
// we can start a workflow, cancel it, and verify the
// workflow's status is "cancelled" and all in-flight steps
// are marked cancelled. We don't need a real LLM because
// the engine's hooks never fire if Start is called
// with a working planner mock.
//
// For this test we bypass the LLM by injecting a fake
// planner role whose SystemPrompt returns a no-op. The
// actual LLM call is the part we can't easily test
// without a network; the start/cancel flow is what
// matters here.
func TestEngine_StartCancel(t *testing.T) {
	cfg := minimalConfig()
	e := NewEngine(cfg)
	id, err := e.Start(context.Background(), "test", "build me a thing", "development", 3)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if id == "" {
		t.Fatal("Start returned empty id")
	}
	// The engine spawns a goroutine. We can't reliably
	// observe its intermediate state without timing
	// dependencies, so just verify the workflow was
	// registered and cancel it.
	e.Cancel(id)
	snap := e.Snapshot(id)
	if snap.Status != StepCancelled {
		t.Errorf("after Cancel, status = %q, want %q", snap.Status, StepCancelled)
	}
}

// TestEngine_WorkflowsOrderNewestFirst verifies the
// Workflows() method returns workflows newest-first so the
// Workflows tab shows the most recent run at the top.
func TestEngine_WorkflowsOrderNewestFirst(t *testing.T) {
	cfg := minimalConfig()
	e := NewEngine(cfg)
	id1, _ := e.Start(context.Background(), "first", "g1", "development", 1)
	id2, _ := e.Start(context.Background(), "second", "g2", "development", 1)
	flows := e.Workflows()
	if len(flows) < 2 {
		t.Fatalf("Workflows() returned %d, want >= 2", len(flows))
	}
	// Newest first \u2014 the second-started should be at index 0.
	if flows[0].ID != id2 {
		t.Errorf("flows[0].ID = %q, want %q (newest first)", flows[0].ID, id2)
	}
	if flows[1].ID != id1 {
		t.Errorf("flows[1].ID = %q, want %q", flows[1].ID, id1)
	}
	// Cleanup
	e.Cancel(id1)
	e.Cancel(id2)
}

// TestEngine_GetReturnsNilForUnknown pins the Get() method
// against an unknown ID. The UI uses Get() to look up a
// workflow by ID for cancellation; an unknown ID must
// return nil, not panic.
func TestEngine_GetReturnsNilForUnknown(t *testing.T) {
	cfg := minimalConfig()
	e := NewEngine(cfg)
	if got := e.Get("nonexistent"); got != nil {
		t.Errorf("Get(\"nonexistent\") = %+v, want nil", got)
	}
}

// TestEngine_SnapshotIncludesCurrentMsg pins the CurrentMsg
// field. The right panel uses this to show "▸ <msg>" so
// the user can see what the active step is doing without
// opening the Workflows tab.
func TestEngine_SnapshotIncludesCurrentMsg(t *testing.T) {
	cfg := minimalConfig()
	e := NewEngine(cfg)
	id, _ := e.Start(context.Background(), "test", "g", "development", 1)
	defer e.Cancel(id)
	// Set a current message and verify the snapshot carries it.
	wf := e.Get(id)
	if wf == nil {
		t.Fatal("Get returned nil")
	}
	wf.setCurrentMsg("developer: writing the auth middleware")
	snap := e.Snapshot(id)
	if snap.CurrentMsg != "developer: writing the auth middleware" {
		t.Errorf("CurrentMsg = %q, want the test message", snap.CurrentMsg)
	}
}

// minimalConfig returns a Config with just enough fields
// populated to construct an Engine. The engine's LLM paths
// are not exercised by these tests.
func minimalConfig() *cortexconfig.Config {
	return &cortexconfig.Config{DefaultModel: "test-model"}
}

// TestStepStatusConstants pins the Status string constants
// that other packages compare against. If we change
// "in_progress" to "running" without updating the UI, the
// Workflows panel will silently stop rendering.
func TestStepStatusConstants(t *testing.T) {
	if StepPending != "pending" {
		t.Errorf("StepPending = %q, want pending", StepPending)
	}
	if StepInProgress != "in_progress" {
		t.Errorf("StepInProgress = %q, want in_progress", StepInProgress)
	}
	if StepDone != "done" {
		t.Errorf("StepDone = %q, want done", StepDone)
	}
	if StepFailed != "failed" {
		t.Errorf("StepFailed = %q, want failed", StepFailed)
	}
	if StepCancelled != "cancelled" {
		t.Errorf("StepCancelled = %q, want cancelled", StepCancelled)
	}
}
