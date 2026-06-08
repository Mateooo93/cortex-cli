package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeHexColor(t *testing.T) {
	got, err := NormalizeHexColor("3b82f6")
	if err != nil {
		t.Fatal(err)
	}
	if got != "#3B82F6" {
		t.Errorf("got %q, want #3B82F6", got)
	}
	if _, err := NormalizeHexColor("not-a-color"); err == nil {
		t.Fatal("expected error for invalid color")
	}
	if got, err := NormalizeHexColor(""); err != nil || got != "" {
		t.Fatalf("empty = %q, %v", got, err)
	}
}

func TestSetThemeColors_PersistsToHomeSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := SetThemeColors("#8B5CF6", "#00FFAA"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, ".cortex", "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Theme ThemeConfig `json:"theme"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Primary != "#8B5CF6" {
		t.Errorf("primary = %q", cfg.Theme.Primary)
	}
	if cfg.Theme.Secondary != "" {
		t.Errorf("secondary = %q, want cleared", cfg.Theme.Secondary)
	}

	paths := NewCortexPaths("", filepath.Join(home, ".cortex"), t.TempDir())
	tc := LoadThemeConfig(paths)
	if tc.Primary != "#8B5CF6" || tc.Secondary != "" {
		t.Errorf("LoadThemeConfig() = %+v", tc)
	}
}

func TestNextThemeColorPreset_CyclesPresets(t *testing.T) {
	p := NextThemeColorPreset("")
	if p != "#8B5CF6" {
		t.Fatalf("default -> violet = %q", p)
	}
	p = NextThemeColorPreset(p)
	if p != "#10B981" {
		t.Fatalf("violet -> emerald = %q", p)
	}
	p = NextThemeColorPreset("#6366F1")
	if p != "" {
		t.Fatalf("indigo -> default = %q", p)
	}
}

func TestThemeColorPresetName(t *testing.T) {
	if got := ThemeColorPresetName("#8B5CF6"); got != "violet" {
		t.Fatalf("got %q, want violet", got)
	}
	if got := ThemeColorPresetName(""); got != "default" {
		t.Fatalf("got %q, want default", got)
	}
}

func TestSetThemeColors_ClearsSecondaryOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := SetThemeColors("#8B5CF6", "#00FFAA"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, ".cortex", "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"secondary"`) {
		t.Errorf("expected secondary override cleared, got %s", string(data))
	}
}

func TestSetThemeColors_ClearRestoresDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := SetThemeColors("#8B5CF6", "#222222"); err != nil {
		t.Fatal(err)
	}
	if err := SetThemeColors("", ""); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, ".cortex", "settings.json")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"theme"`) {
		t.Errorf("expected theme block removed, got %s", string(data))
	}
}

func TestSetThemeColors_RejectsNonPresetPrimary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := SetThemeColors("#FF00AA", ""); err == nil {
		t.Fatal("expected error for non-preset primary")
	}
}

func TestLoadThemeConfig_IgnoresProjectTheme(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	homeSettings := filepath.Join(home, ".cortex")
	if err := os.MkdirAll(homeSettings, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeSettings, "settings.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	projectSettings := filepath.Join(project, ".cortex")
	if err := os.MkdirAll(projectSettings, 0o755); err != nil {
		t.Fatal(err)
	}
	projectData := `{"theme":{"primary":"#F43F5E","secondary":"#00FF00"}}`
	if err := os.WriteFile(filepath.Join(projectSettings, "settings.json"), []byte(projectData), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := NewCortexPaths("", filepath.Join(home, ".cortex"), project)
	tc := LoadThemeConfig(paths)
	if tc.Primary != "" || tc.Secondary != "" {
		t.Errorf("LoadThemeConfig() = %+v, want defaults (project theme ignored)", tc)
	}
}

func TestLoadThemeConfig_IgnoresUnknownPrimary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	homeSettings := filepath.Join(home, ".cortex")
	if err := os.MkdirAll(homeSettings, 0o755); err != nil {
		t.Fatal(err)
	}
	homeData := `{"theme":{"primary":"#FF00AA","secondary":"#00FF00"}}`
	if err := os.WriteFile(filepath.Join(homeSettings, "settings.json"), []byte(homeData), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := NewCortexPaths("", filepath.Join(home, ".cortex"), t.TempDir())
	tc := LoadThemeConfig(paths)
	if tc.Primary != "" || tc.Secondary != "" {
		t.Errorf("LoadThemeConfig() = %+v, want defaults (unknown primary ignored)", tc)
	}
}