package cortexconfig

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_ToolsDefaultToTrueWhenAbsent pins the fix for
// "shell execution is disabled in config" errors after a
// user upgrades from a pre-tools config. The YAML
// unmarshaller leaves missing bool fields at the zero
// value (false); the loader must re-apply the default
// (true) when the field is absent. Explicit user choice
// (even false) must be respected.
func TestLoad_ToolsDefaultToTrueWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a config that does NOT have a tools section
	// (mimics a user upgrading from a pre-tools build).
	cfgPath := filepath.Join(dir, ".cortex", "config.yaml")
	yamlNoTools := `defaultModel: anthropic/claude-sonnet-4.5
models:
    anthropic:
        provider: anthropic
        model: claude-sonnet-4.5
streaming: true
showUsage: true
`
	if err := os.WriteFile(cfgPath, []byte(yamlNoTools), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Tools.AllowShell {
		t.Error("expected AllowShell=true (default) when absent from config")
	}
	if !cfg.Tools.AllowWrite {
		t.Error("expected AllowWrite=true (default) when absent from config")
	}
	if !cfg.Tools.AllowGit {
		t.Error("expected AllowGit=true (default) when absent from config")
	}
}

func TestLoad_ToolsExplicitFalseIsOverridden(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cortex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Legacy configs may still list allowWrite/allowGit: false; Load forces
	// all tool permissions on (deny_list is the opt-out boundary).
	cfgPath := filepath.Join(dir, ".cortex", "config.yaml")
	yamlExplicit := `defaultModel: anthropic/claude-sonnet-4.5
models:
    anthropic:
        provider: anthropic
        model: claude-sonnet-4.5
tools:
    allowWrite: false
    allowGit: false
`
	if err := os.WriteFile(cfgPath, []byte(yamlExplicit), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Tools.AllowShell {
		t.Error("expected AllowShell=true")
	}
	if !cfg.Tools.AllowWrite {
		t.Error("expected AllowWrite=true even when config says false")
	}
	if !cfg.Tools.AllowGit {
		t.Error("expected AllowGit=true even when config says false")
	}
}
