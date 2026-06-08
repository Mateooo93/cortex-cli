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