package cortexconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSave_PreservesUnknownFields pins the user-reported
// bug: "i lose my whole provider config when updating".
// The old Save() called yaml.Marshal on the Go struct,
// which dropped any field the struct didn't know about
// (custom provider fields, comments, hand-edited keys).
// The fix: Save now deep-merges the new struct over the
// existing on-disk YAML, so unknown keys survive.
func TestSave_PreservesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(dir, ".cortex", "config.yaml")
	// Simulate a config that has custom fields the Go
	// struct doesn't know about.
	yaml := `defaultModel: anthropic/claude-sonnet-4.5
models:
    anthropic:
        provider: anthropic
        model: claude-sonnet-4.5
        baseUrl: https://api.anthropic.com/v1
        customHeader: "x-my-thing: bar"
        extraStuff:
            sub: 1
    custom-provider:
        provider: myproxy
        model: gpt-4
        madeUpField: hello
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Simulate the user picking a different default
	// model — this is the path that triggered Save() in
	// the model picker and clobbered the config.
	cfg.DefaultModel = "anthropic/claude-haiku-4-5"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(out)
	// The customHeader on the anthropic model must
	// survive the save.
	if !strings.Contains(got, "customHeader") {
		t.Errorf("customHeader was dropped by Save: %s", got)
	}
	// The custom-provider entry must survive.
	if !strings.Contains(got, "custom-provider") {
		t.Errorf("custom-provider was dropped by Save: %s", got)
	}
	if !strings.Contains(got, "madeUpField") {
		t.Errorf("madeUpField on custom-provider was dropped by Save: %s", got)
	}
	// The new default model must be written.
	if !strings.Contains(got, "defaultModel: anthropic/claude-haiku-4-5") {
		t.Errorf("new default model not written: %s", got)
	}
}

// TestSave_PreservesExistingComments pins a related
// concern: the old Save would strip YAML comments. The
// merge approach can't preserve comments (yaml.Unmarshal
// drops them) but at least it doesn't re-serialize the
// file in a way that destroys user-added custom
// structure. (Comment preservation would require a
// yaml.Node-based approach which is out of scope for the
// fix.)
func TestSave_OnlyUpdatesRequestedField(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(dir, ".cortex", "config.yaml")
	yaml := `defaultModel: anthropic/claude-sonnet-4.5
streaming: false
showUsage: false
autoCompact: true
models:
    anthropic:
        provider: anthropic
        model: claude-sonnet-4.5
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// User explicitly toggles streaming ON.
	cfg.Streaming = true
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, _ := os.ReadFile(cfgPath)
	got := string(out)
	// The user's explicit choices for showUsage and
	// autoCompact must NOT be reset by Save.
	if !strings.Contains(got, "showUsage: false") {
		t.Errorf("showUsage: false was clobbered by Save: %s", got)
	}
	if !strings.Contains(got, "autoCompact: true") {
		t.Errorf("autoCompact: true was clobbered by Save: %s", got)
	}
	if !strings.Contains(got, "streaming: true") {
		t.Errorf("streaming: true was not written: %s", got)
	}
}
