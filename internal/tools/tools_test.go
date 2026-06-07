package tools

import (
	"os/exec"
	"strings"
	"testing"
)

// TestShellCommandPrefersBash verifies that the shell tool defaults
// to bash (which gives consistent behaviour across macOS/Linux)
// and only falls back to sh on systems without bash. The user
// complained that the old `sh -c` invocation was unreliable for
// common one-liners, especially on macOS where /bin/sh is a
// strict POSIX dash.
func TestShellCommandPrefersBash(t *testing.T) {
	got := shellCommand()
	if _, err := exec.LookPath("bash"); err == nil {
		if got != "bash" {
			t.Errorf("expected shellCommand to return bash when bash is installed, got %q", got)
		}
	} else {
		if got != "sh" {
			t.Errorf("expected shellCommand to fall back to sh when bash is missing, got %q", got)
		}
	}
}

// TestShellToolRunsCommand verifies the shell tool actually
// executes commands and returns their output.
func TestShellToolRunsCommand(t *testing.T) {
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("no /bin/echo available; cannot exercise the shell tool")
	}
	tctx := Context{CWD: t.TempDir(), AllowShell: true}
	res, err := (&ShellTool{}).Run(tctx, map[string]any{
		"command": "echo hello-from-tool",
	})
	if err != nil {
		t.Fatalf("shell tool returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("shell tool result not OK: %s", res.Error)
	}
	if !strings.Contains(res.Output, "hello-from-tool") {
		t.Errorf("expected output to contain 'hello-from-tool', got %q", res.Output)
	}
}

// TestShellToolBashSubstitutionWorks verifies that a bash-only
// variable expansion (${VAR,,} for lowercase) works through the
// shell tool. Under dash/sh without bash, this expansion is
// silently left as the literal string, which is the exact
// failure mode the user reported.
func TestShellToolBashSubstitutionWorks(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed on this system")
	}
	if _, err := exec.LookPath("echo"); err != nil {
		t.Skip("no /bin/echo available")
	}
	tctx := Context{CWD: t.TempDir(), AllowShell: true}
	res, err := (&ShellTool{}).Run(tctx, map[string]any{
		"command": `BASH_TEST=HELLO && echo "${BASH_TEST,,}"`,
	})
	if err != nil {
		t.Fatalf("shell tool returned error: %v", err)
	}
	if !res.OK {
		t.Fatalf("shell tool result not OK: %s", res.Error)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("expected bash to lowercase the variable, got %q (the literal %q expansion is bash-specific)", res.Output, "${BASH_TEST,,}")
	}
}

// TestDispatchWorkflowTool_RequiresPrompt verifies the
// dispatch_workflow tool rejects empty prompts. The user
// reported: "the agent isnt using workflows" — so we want
// the tool to be wired correctly and fail loudly on
// misuse.
func TestDispatchWorkflowTool_RequiresPrompt(t *testing.T) {
	tctx := Context{CWD: t.TempDir()}
	res, err := (&DispatchWorkflowTool{}).Run(tctx, map[string]any{
		"prompt": "",
	})
	if err != nil {
		t.Fatalf("dispatch_workflow returned error: %v", err)
	}
	if res.OK {
		t.Error("expected dispatch_workflow to fail on empty prompt")
	}
	if !strings.Contains(res.Error, "prompt") {
		t.Errorf("expected error to mention 'prompt', got %q", res.Error)
	}
}

// TestDispatchWorkflowTool_AcceptsValidPrompt verifies
// the tool returns a confirmation marker for a valid
// prompt. The session handler picks up the tool call
// and emits the EventWorkflowDispatch that the TUI uses
// to actually start the orchestrator.
func TestDispatchWorkflowTool_AcceptsValidPrompt(t *testing.T) {
	tctx := Context{CWD: t.TempDir()}
	res, err := (&DispatchWorkflowTool{}).Run(tctx, map[string]any{
		"prompt": "build a CLI todo app in Go with a test suite",
	})
	if err != nil {
		t.Fatalf("dispatch_workflow returned error: %v", err)
	}
	if !res.OK {
		t.Errorf("expected dispatch_workflow to succeed, got error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "workflow dispatched") {
		t.Errorf("expected 'workflow dispatched' in output, got %q", res.Output)
	}
	if !strings.Contains(res.Output, "build a CLI todo app") {
		t.Errorf("expected prompt to be echoed in output, got %q", res.Output)
	}
}

// TestDispatchWorkflowTool_AcceptsPreset verifies the
// preset parameter is captured in the output. The UI
// handler parses "preset=X ..." and routes to the
// matching BuiltinPreset (code / research / test /
// review / docs).
func TestDispatchWorkflowTool_AcceptsPreset(t *testing.T) {
	tctx := Context{CWD: t.TempDir()}
	res, err := (&DispatchWorkflowTool{}).Run(tctx, map[string]any{
		"prompt": "compare postgres vs sqlite for our use case",
		"preset": "research",
	})
	if err != nil {
		t.Fatalf("dispatch_workflow returned error: %v", err)
	}
	if !res.OK {
		t.Errorf("expected dispatch_workflow to succeed, got error: %s", res.Error)
	}
	if !strings.Contains(res.Output, "preset=research") {
		t.Errorf("expected 'preset=research' in output, got %q", res.Output)
	}
}

// TestDispatchWorkflowTool_RegisteredInDefaultSet
// verifies the tool is wired into the default tool
// registry. Without this, the LLM has no way to call
// it and the user-reported bug ("the agent isnt using
// workflows") would persist.
func TestDispatchWorkflowTool_RegisteredInDefaultSet(t *testing.T) {
	r := NewRegistry()
	tool, ok := r.Get("dispatch_workflow")
	if !ok {
		t.Fatal("dispatch_workflow is not registered in the default toolset")
	}
	if tool.Name() != "dispatch_workflow" {
		t.Errorf("tool name = %q, want 'dispatch_workflow'", tool.Name())
	}
	// Also verify the LLM-facing description is
	// substantial (the agent only learns to use it
	// from the description, so a one-liner would
	// not work).
	if len(tool.Description()) < 100 {
		t.Errorf("dispatch_workflow description is too short (%d chars); the agent only learns the tool from this", len(tool.Description()))
	}
}
