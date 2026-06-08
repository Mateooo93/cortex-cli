package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasb-eyer/go-colorful"
)

// Default brand colors (match internal/ui/styles.go).
const (
	DefaultThemePrimary   = "#3B82F6"
	DefaultThemeSecondary = "#60A5FA"
)

// NormalizeHexColor validates and canonicalizes a #RRGGBB color. Empty input
// means "use the built-in default" (caller clears the stored override).
func NormalizeHexColor(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "#") {
		raw = "#" + raw
	}
	if len(raw) != 7 {
		return "", fmt.Errorf("expected #RRGGBB, got %q", raw)
	}
	c, err := colorful.Hex(raw)
	if err != nil {
		return "", fmt.Errorf("invalid hex color %q", raw)
	}
	return strings.ToUpper(c.Hex()), nil
}

// EffectiveThemePrimary returns the configured primary or the default.
func (tc ThemeConfig) EffectivePrimary() string {
	if tc.Primary != "" {
		return tc.Primary
	}
	return DefaultThemePrimary
}

// EffectiveSecondary returns the configured secondary or the default.
func (tc ThemeConfig) EffectiveSecondary() string {
	if tc.Secondary != "" {
		return tc.Secondary
	}
	return DefaultThemeSecondary
}

// SetThemeColors writes primary/secondary colors to ~/.cortex/settings.json so
// they survive binary updates and reinstalls. Empty strings remove overrides and
// restore the built-in defaults on next launch.
func SetThemeColors(primary, secondary string) error {
	primary, err := NormalizeHexColor(primary)
	if err != nil {
		return err
	}
	secondary, err = NormalizeHexColor(secondary)
	if err != nil {
		return err
	}

	home := HomeCortexDir()
	if home == "" {
		return fmt.Errorf("no home directory")
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	p := filepath.Join(home, "settings.json")

	raw := map[string]any{}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &raw)
	}

	theme, _ := raw["theme"].(map[string]any)
	if theme == nil {
		theme = map[string]any{}
	}
	if primary == "" {
		delete(theme, "primary")
	} else {
		theme["primary"] = primary
	}
	if secondary == "" {
		delete(theme, "secondary")
	} else {
		theme["secondary"] = secondary
	}
	if len(theme) == 0 {
		delete(raw, "theme")
	} else {
		raw["theme"] = theme
	}

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, out, 0o644)
}