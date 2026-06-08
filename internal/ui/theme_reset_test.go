package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
)

func TestResetThemeColors_RestoresDefaultBlue(t *testing.T) {
	m := NewModel(&config.Config{}, nil, nil, true, "", true, true)
	m.themeColors = config.ThemeConfig{Primary: "#8B5CF6"}

	if err := m.resetThemeColors(); err != nil {
		t.Fatal(err)
	}
	if m.themeColors.Primary != "" {
		t.Fatalf("primary = %q, want empty (default)", m.themeColors.Primary)
	}
	if primaryHex != config.DefaultThemePrimary {
		t.Fatalf("primaryHex = %q, want %q", primaryHex, config.DefaultThemePrimary)
	}
}