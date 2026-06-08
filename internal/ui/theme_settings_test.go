package ui

import (
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
)

func TestApplyConfiguredTheme_UsesStoredColors(t *testing.T) {
	m := NewModel(&config.Config{}, nil, nil, true, "", true, true)
	m.themeColors = config.ThemeConfig{Primary: "#FF0000", Secondary: "#00FF00"}
	m.applyConfiguredTheme()
	if primaryHex != "#FF0000" {
		t.Errorf("primaryHex = %q, want #FF0000", primaryHex)
	}
	if secondaryHex != "#00FF00" {
		t.Errorf("secondaryHex = %q, want #00FF00", secondaryHex)
	}
}