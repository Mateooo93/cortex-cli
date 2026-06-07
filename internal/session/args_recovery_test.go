package session

import (
	"strings"
	"testing"
)

// TestRecoverArgsFromRaw_TruncatedWriteFile pins the fix
// for the "ERROR: path and content are required" error
// the user reported. The LLM emits a tool call whose
// JSON arguments exceed the output token budget, the
// provider stores the raw partial string in
// `args["_raw"]`, and the tool rejects the call. We
// recover the path + content from the raw string and
// the file actually gets written.
func TestRecoverArgsFromRaw_TruncatedWriteFile(t *testing.T) {
	// Simulate the provider fallback: the JSON didn't
	// parse so we got `_raw` containing the partial
	// arguments string.
	raw := `{"path": "icons.css", "content": "/* big icon library */\n.icon-home { ... }`
	args := map[string]any{"_raw": raw}

	recovered := recoverArgsFromRaw("write_file", args, raw)
	path, _ := recovered["path"].(string)
	content, _ := recovered["content"].(string)
	if path != "icons.css" {
		t.Errorf("recovered path = %q, want 'icons.css'", path)
	}
	if !strings.HasPrefix(content, "/* big icon library */") {
		t.Errorf("recovered content has wrong prefix: %q", content)
	}
	if !strings.Contains(content, ".icon-home") {
		t.Errorf("recovered content is missing the icon class: %q", content)
	}
}

// TestRecoverArgsFromRaw_TruncatedShell verifies shell
// command recovery. The user reported
// "ERROR: command is required" — the same root cause
// (truncated tool-call JSON for a very long command).
func TestRecoverArgsFromRaw_TruncatedShell(t *testing.T) {
	raw := `{"command": "echo hello && cat /etc/issue && uname -a && uptime`
	args := map[string]any{"_raw": raw}

	recovered := recoverArgsFromRaw("run_shell", args, raw)
	cmd, _ := recovered["command"].(string)
	if !strings.HasPrefix(cmd, "echo hello") {
		t.Errorf("recovered command has wrong prefix: %q", cmd)
	}
	if !strings.Contains(cmd, "uname -a") {
		t.Errorf("recovered command is missing 'uname -a': %q", cmd)
	}
}

// TestRecoverArgsFromRaw_NoRaw verifies the helper is a
// no-op when there's no _raw field (the normal happy
// path — the JSON parsed fine).
func TestRecoverArgsFromRaw_NoRaw(t *testing.T) {
	args := map[string]any{"path": "foo.txt", "content": "hello"}
	recovered := recoverArgsFromRaw("write_file", args, "")
	if recovered["path"] != "foo.txt" {
		t.Errorf("expected untouched path, got %q", recovered["path"])
	}
	if recovered["content"] != "hello" {
		t.Errorf("expected untouched content, got %q", recovered["content"])
	}
}

// TestRecoverArgsFromRaw_UnknownTool verifies the helper
// doesn't crash on tools it doesn't know about — it
// just returns the args unchanged.
func TestRecoverArgsFromRaw_UnknownTool(t *testing.T) {
	raw := `{"foo": "bar"}`
	args := map[string]any{"_raw": raw, "existing": "value"}
	recovered := recoverArgsFromRaw("some_random_tool", args, raw)
	if recovered["existing"] != "value" {
		t.Errorf("expected existing=value to be preserved, got %q", recovered["existing"])
	}
	// For unknown tools, we should NOT add the foo
	// key from the raw string (we only know how to
	// extract fields for specific tools).
	if _, ok := recovered["foo"]; ok {
		t.Error("expected 'foo' NOT to be extracted for unknown tool")
	}
}

// TestRecoverArgsFromRaw_FullyClosedJSON verifies that
// properly closed JSON also recovers correctly. This is
// the case where JSON.parse failed for some OTHER reason
// (e.g. trailing comma) but the field values are
// complete.
func TestRecoverArgsFromRaw_FullyClosedJSON(t *testing.T) {
	raw := `{"path": "style.css", "content": "body { color: red; }"}`
	args := map[string]any{"_raw": raw}
	recovered := recoverArgsFromRaw("write_file", args, raw)
	path, _ := recovered["path"].(string)
	content, _ := recovered["content"].(string)
	if path != "style.css" {
		t.Errorf("recovered path = %q, want 'style.css'", path)
	}
	if content != "body { color: red; }" {
		t.Errorf("recovered content = %q, want 'body { color: red; }'", content)
	}
}
