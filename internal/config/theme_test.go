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

	if err := SetThemeColors("#FF00AA", "#00FFAA"); err != nil {
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
	if cfg.Theme.Primary != "#FF00AA" {
		t.Errorf("primary = %q", cfg.Theme.Primary)
	}
	if cfg.Theme.Secondary != "#00FFAA" {
		t.Errorf("secondary = %q", cfg.Theme.Secondary)
	}

	paths := NewCortexPaths("", filepath.Join(home, ".cortex"), t.TempDir())
	tc := LoadThemeConfig(paths)
	if tc.Primary != "#FF00AA" || tc.Secondary != "#00FFAA" {
		t.Errorf("LoadThemeConfig() = %+v", tc)
	}
}

func TestNextThemeColorPreset_CyclesPresets(t *testing.T) {
	p, s := NextThemeColorPreset("", "")
	if p != "#8B5CF6" || s != "#A78BFA" {
		t.Fatalf("default -> violet = %q, %q", p, s)
	}
	p, s = NextThemeColorPreset(p, s)
	if p != "#10B981" || s != "#34D399" {
		t.Fatalf("violet -> emerald = %q, %q", p, s)
	}
	p, s = NextThemeColorPreset("#6366F1", "#818CF8")
	if p != "" || s != "" {
		t.Fatalf("indigo -> default = %q, %q", p, s)
	}
}

func TestThemeColorPresetName(t *testing.T) {
	if got := ThemeColorPresetName("#8B5CF6", "#A78BFA"); got != "violet" {
		t.Fatalf("got %q, want violet", got)
	}
	if got := ThemeColorPresetName("", ""); got != "default" {
		t.Fatalf("got %q, want default", got)
	}
}

func TestSetThemeColors_ClearRestoresDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := SetThemeColors("#111111", "#222222"); err != nil {
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