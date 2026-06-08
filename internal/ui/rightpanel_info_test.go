package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestModelContextWindow covers the lookup table for known
// models. The right panel and slim footer use this to render
// the context-usage bar; if the window is wrong (e.g. 4096 for
// a 200k model) the bar will fill up at 2% and look broken.
func TestModelContextWindow(t *testing.T) {
	tests := []struct {
		spec string
		want int64
	}{
		// OpenAI
		{"openai:gpt-5", 400_000},
		{"openai:gpt-5-mini", 400_000},
		{"openai:gpt-4.1", 1_000_000},
		{"openai:gpt-4o", 128_000},
		{"openai:o3", 200_000},
		{"openai:o4-mini", 200_000},
		// Anthropic
		{"anthropic:claude-opus-4-8", 200_000},
		{"anthropic:claude-sonnet-4-5", 200_000},
		{"anthropic:claude-3-5-sonnet", 200_000},
		// Google
		{"google:gemini-2.5-pro", 1_000_000},
		{"google:gemini-1.5-pro", 2_000_000},
		// Mistral / DeepSeek / Qwen
		{"mistral:mistral-large", 128_000},
		{"deepseek:deepseek-chat", 128_000},
		// Local / unknown
		{"ollama:llama3.1", 0},
		{"", 0},
		{"openai:unknown-model", 0},
	}
	for _, tt := range tests {
		got := cortexconfig.ModelContextWindow(tt.spec)
		if got != tt.want {
			t.Errorf("ModelContextWindow(%q) = %d, want %d", tt.spec, got, tt.want)
		}
	}
}

// TestRightPanel_InfoMode_ShowsContextBar verifies the info
// panel renders the context-usage bar with the correct
// percentage. This is the headline user-facing metric the user
// asked for.
func TestRightPanel_OutlineMatchesSettingsViewport(t *testing.T) {
	s := NewStyles(true)
	if rightPanelBorderStyle(s).GetBorderLeftForeground() != s.ViewportFocusedStyle.GetBorderLeftForeground() {
		t.Fatal("right panel should use the same border style as Settings")
	}
	if rightPanelBorderStyle(s).GetBorderTop() {
		t.Fatal("right panel should omit top border like Settings viewport")
	}
}

func TestRightPanel_InfoMode_ShowsContextBar(t *testing.T) {
	rp := RightPanel{}
	rp.OpenInfo(40)
	s := NewStyles(true)
	info := RightPanelInfoView{
		ModelName:   "GPT-5.5",
		ProviderName: "ChatGPT (codex)",
		InputTokens: 12_000,
		ContextMax:  200_000,
		Elapsed:     2*time.Minute + 13*time.Second,
		Connected:   true,
	}
	view := rp.View(40, s, true, "GPT-5.5", nil, info)
	// Should contain "Model", "Context", "6%" (12k/200k),
	for _, want := range []string{
		"Model", "Context", "6%", "2:13", "Keys",
		"Ctrl+T", "Ctrl+B", "Ctrl+C", "/",
		"Esc close panel",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("info panel missing %q, got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"F1", "F2", "F4", "Tab", "Enter", "send", "queue", "cancel"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("info panel should not contain %q (moved to tab bar / input hints), got:\n%s", unwanted, view)
		}
	}
	if strings.Contains(view, "connected") {
		t.Errorf("info panel should not show connection status, got:\n%s", view)
	}
}

// TestRightPanel_InfoMode_NoContextWindow covers the case
// where the model isn't in the lookup table (local model,
// custom provider). The bar should still render but without
// a percentage label, so the user can see they're using
// 12k tokens but the bar doesn't claim "100%" or similar.
func TestRightPanel_InfoMode_NoContextWindow(t *testing.T) {
	rp := RightPanel{}
	rp.OpenInfo(20)
	s := NewStyles(true)
	info := RightPanelInfoView{
		ModelName:   "llama3.1",
		ProviderName: "ollama",
		InputTokens: 12_000,
		ContextMax:  0,    // unknown
		Connected:   true,
	}
	view := rp.View(40, s, true, "llama3.1", nil, info)
	if !strings.Contains(view, "12k") {
		t.Errorf("expected '12k' token count, got:\n%s", view)
	}
	if strings.Contains(view, "100%") {
		t.Errorf("expected NO percentage when window unknown, got:\n%s", view)
	}
	if !strings.Contains(view, "window unknown") {
		t.Errorf("expected 'window unknown' label, got:\n%s", view)
	}
}

// TestRightPanel_InfoMode_AutoCompactWarning verifies the
// panel shows a warning when auto-compact is on and the user
// is above the 80% threshold. This is the visual signal the
// user gets before auto-compact fires on the next turn.
func TestRightPanel_InfoMode_AutoCompactWarning(t *testing.T) {
	rp := RightPanel{}
	rp.OpenInfo(20)
	s := NewStyles(true)
	info := RightPanelInfoView{
		ModelName:   "GPT-5.5",
		InputTokens: 170_000,  // 85% of 200k
		ContextMax:  200_000,
		AutoCompact: true,
	}
	view := rp.View(40, s, true, "GPT-5.5", nil, info)
	if !strings.Contains(view, "auto-compact") {
		t.Errorf("expected auto-compact warning, got:\n%s", view)
	}
	if !strings.Contains(view, "85%") {
		t.Errorf("expected 85%% context percentage, got:\n%s", view)
	}
}
