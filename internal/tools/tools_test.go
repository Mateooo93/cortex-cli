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
func TestParseEditFileEdits_RequiresNewStringWithLegacyOldString(t *testing.T) {
	_, err := parseEditFileEdits(map[string]any{
		"path":      "main.go",
		"oldString": "\tfmt.Println(\"hello\")",
	})
	if err == nil {
		t.Fatal("expected error when oldString is set without newString")
	}
	if !strings.Contains(err.Error(), "newString is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEditFileEdits_AllowsExplicitEmptyNewString(t *testing.T) {
	edits, err := parseEditFileEdits(map[string]any{
		"path":      "main.go",
		"oldString": "todo",
		"newString": "",
	})
	if err != nil {
		t.Fatalf("explicit empty newString should be allowed: %v", err)
	}
	if len(edits) != 1 || edits[0].NewText != "" {
		t.Fatalf("unexpected edits: %+v", edits)
	}
}

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
