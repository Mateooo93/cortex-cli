package config

import (
	"path/filepath"
	"testing"
)

func TestCortexPaths_NormalMode(t *testing.T) {
	p := NewCortexPaths("", "/home/.cortex", "/project")

	if p.IsOverride() {
		t.Error("expected IsOverride false in normal mode")
	}

	layers := p.Layers()
	want := []string{"/home/.cortex", filepath.Join("/project", ".cortex")}
	if len(layers) != len(want) {
		t.Fatalf("Layers length = %d, want %d", len(layers), len(want))
	}
	for i, w := range want {
		if layers[i] != w {
			t.Errorf("Layers[%d] = %q, want %q", i, layers[i], w)
		}
	}

	if got := p.Primary(); got != filepath.Join("/project", ".cortex") {
		t.Errorf("Primary = %q, want %q", got, filepath.Join("/project", ".cortex"))
	}

	if got := p.Logs(); got != filepath.Join("/home/.cortex", "logs") {
		t.Errorf("Logs = %q, want %q", got, filepath.Join("/home/.cortex", "logs"))
	}

	if got := p.AccessStatsDB(); got != filepath.Join("/project", ".cortex", "access_stats.db") {
		t.Errorf("AccessStatsDB = %q", got)
	}

	if got := p.MemoryDB(); got != filepath.Join("/project", ".cortex", "memory.db") {
		t.Errorf("MemoryDB = %q", got)
	}
	if got := p.ContextMD(); got != filepath.Join("/project", ".cortex", "context.md") {
		t.Errorf("ContextMD = %q", got)
	}

	if got := p.History(); got != filepath.Join("/project", ".cortex", "history.txt") {
		t.Errorf("History = %q", got)
	}

	settings := p.Settings()
	if len(settings) != 2 {
		t.Fatalf("Settings length = %d, want 2", len(settings))
	}
	if settings[1] != filepath.Join("/project", ".cortex", "settings.json") {
		t.Errorf("project settings = %q", settings[1])
	}

	claudeMD := p.ClaudeMD()
	// Normal mode: [home/CLAUDE.md, cwd/CLAUDE.md]
	if len(claudeMD) != 2 {
		t.Fatalf("ClaudeMD length = %d, want 2", len(claudeMD))
	}
	if claudeMD[1] != filepath.Join("/project", "CLAUDE.md") {
		t.Errorf("ClaudeMD[1] = %q, want cwd CLAUDE.md", claudeMD[1])
	}
}

func TestCortexPaths_OverrideMode(t *testing.T) {
	p := NewCortexPaths("/custom", "/home/.cortex", "/project")

	if !p.IsOverride() {
		t.Error("expected IsOverride true")
	}

	if p.Home() != "" {
		t.Errorf("Home should be empty in override mode, got %q", p.Home())
	}
	if p.Project() != "" {
		t.Errorf("Project should be empty in override mode, got %q", p.Project())
	}

	layers := p.Layers()
	if len(layers) != 1 || layers[0] != "/custom" {
		t.Errorf("Layers = %v, want [/custom]", layers)
	}

	settings := p.Settings()
	if len(settings) != 1 || settings[0] != filepath.Join("/custom", "settings.json") {
		t.Errorf("Settings = %v", settings)
	}

	agents := p.Agents()
	if len(agents) != 1 || agents[0] != filepath.Join("/custom", "agents") {
		t.Errorf("Agents = %v", agents)
	}

	if got := p.Primary(); got != "/custom" {
		t.Errorf("Primary = %q, want /custom", got)
	}

	if got := p.Logs(); got != filepath.Join("/custom", "logs") {
		t.Errorf("Logs = %q", got)
	}

	if got := p.AccessStatsDB(); got != filepath.Join("/custom", "access_stats.db") {
		t.Errorf("AccessStatsDB = %q", got)
	}

	if got := p.History(); got != filepath.Join("/custom", "history.txt") {
		t.Errorf("History = %q", got)
	}

	if got := p.Brain(); got != "/custom" {
		t.Errorf("Brain = %q, want /custom", got)
	}

	if got := p.ProjectSettingsWrite(); got != filepath.Join("/custom", "settings.json") {
		t.Errorf("ProjectSettingsWrite = %q", got)
	}

	claudeMD := p.ClaudeMD()
	if len(claudeMD) != 1 || claudeMD[0] != filepath.Join("/custom", "CLAUDE.md") {
		t.Errorf("ClaudeMD = %v", claudeMD)
	}
}

func TestCortexPaths_NormalModeWithoutHome(t *testing.T) {
	p := NewCortexPaths("", "", "/project")

	layers := p.Layers()
	// Without home, layers is just [project]
	if len(layers) != 1 || layers[0] != filepath.Join("/project", ".cortex") {
		t.Errorf("Layers without home = %v", layers)
	}

	if got := p.Logs(); got != "" {
		t.Errorf("Logs should be empty without home, got %q", got)
	}
}
