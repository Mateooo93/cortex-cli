package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestApplyModelPickerSelection_CodexFiresOAuthWhenUnsigned routes
// unsigned codex picks to the browser OAuth flow (never an API key form).
func TestApplyModelPickerSelection_CodexFiresOAuthWhenUnsigned(t *testing.T) {
	t.Setenv("CODEX_CODEX_TOKEN", "")

	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("codex/gpt-5.5")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(codex) returned nil; should fire OAuth flow")
	}
	if m.settingsWizard.active {
		t.Errorf("wizard became active for codex \u2014 must not (codex is OAuth)")
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Errorf("right-panel key input opened for codex \u2014 must not (codex is OAuth)")
	}
	if m.providerConfigured("codex") {
		if m.currentSettingsModel() != "codex/gpt-5.5" {
			t.Errorf("signed-in codex: active model = %q, want codex/gpt-5.5", m.currentSettingsModel())
		}
	} else if m.currentSettingsModel() != "openai/gpt-5.5" {
		t.Errorf("unsigned codex: active model = %q, want openai/gpt-5.5 until OAuth completes", m.currentSettingsModel())
	}
}

func TestApplyModelPickerSelection_CodexSwitchesWhenSignedIn(t *testing.T) {
	t.Setenv("CODEX_CODEX_TOKEN", "eyJhbGciOiJIUzI1NiJ9.test")

	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("codex/gpt-5.5")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(codex signed-in) returned nil")
	}
	if m.currentSettingsModel() != "codex/gpt-5.5" {
		t.Errorf("active model = %q, want codex/gpt-5.5", m.currentSettingsModel())
	}
}

func TestApplyModelPickerSelection_XaiSubFiresOAuthWhenUnsigned(t *testing.T) {
	t.Setenv("XAI_OAUTH_TOKEN", "")

	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("xai-sub/grok-build")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(xai-sub) returned nil; should fire OAuth flow")
	}
	if m.settingsWizard.active {
		t.Error("wizard became active for xai-sub — must not (subscription OAuth)")
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Error("right-panel key input opened for xai-sub — must not (subscription OAuth)")
	}
}

func TestApplyModelPickerSelection_XaiSubSwitchesWhenSignedIn(t *testing.T) {
	t.Setenv("XAI_OAUTH_TOKEN", "eyJhbGciOiJIUzI1NiJ9.test")

	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("xai-sub/grok-build")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(xai-sub signed-in) returned nil")
	}
	if m.currentSettingsModel() != "xai-sub/grok-build" {
		t.Errorf("active model = %q, want xai-sub/grok-build", m.currentSettingsModel())
	}
}

// TestApplyModelPickerSelection_LocalSwitchesDirectly covers
// the "no key needed" auth kind (Ollama, LM Studio, vLLM).
// Picking a local model should set the active model and
// fire a status message \u2014 no key prompt of any kind.
func TestApplyModelPickerSelection_LocalSwitchesDirectly(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("ollama/qwen3.5")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(ollama) returned nil; should switch model and emit status")
	}
	if m.currentSettingsModel() != "ollama/qwen3.5" {
		t.Errorf("active model = %q, want ollama/qwen3.5", m.currentSettingsModel())
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Errorf("right-panel key input opened for ollama \u2014 must not (local has no key)")
	}
}

// TestApplyModelPickerSelection_EnvSwitchesDirectly covers
// Bedrock (env-var auth). Picking bedrock/anthropic.claude-opus-4-8
// must not open a key form.
func TestApplyModelPickerSelection_EnvSwitchesDirectly(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("bedrock/anthropic.claude-opus-4-8")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(bedrock) returned nil; should switch model and emit status")
	}
	if m.currentSettingsModel() != "bedrock/anthropic.claude-opus-4-8" {
		t.Errorf("active model = %q, want bedrock/anthropic.claude-opus-4-8", m.currentSettingsModel())
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Errorf("right-panel key input opened for bedrock \u2014 must not (env var only)")
	}
}

// TestApplyModelPickerSelection_APIKeyWithStoredKey pins the
// happy path for paid API-key providers: if the user has a key
// already, the picker switches immediately. The new model is
// saved to cortexCfg.DefaultModel.
func TestApplyModelPickerSelection_APIKeyWithStoredKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-1234567890")

	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "anthropic/claude-opus-4-7"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("openai/gpt-5.5")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(openai) returned nil; should switch model and emit status")
	}
	if m.currentSettingsModel() != "openai/gpt-5.5" {
		t.Errorf("active model = %q, want openai/gpt-5.5", m.currentSettingsModel())
	}
	if cfg.DefaultModel != "openai/gpt-5.5" {
		t.Errorf("DefaultModel = %q, want openai/gpt-5.5", cfg.DefaultModel)
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Errorf("right-panel key input opened for openai with stored key \u2014 must not")
	}
}

// TestApplyModelPickerSelection_APIKeyFromSettings ensures keys saved
// via the Settings wizard (cortexCfg.Models) are honored by /model.
func TestApplyModelPickerSelection_APIKeyFromSettings(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	cfg.SetProviderAPIKey("anthropic", "sk-ant-settings-key")
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("anthropic/claude-opus-4-8")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(anthropic) returned nil")
	}
	if m.currentSettingsModel() != "anthropic/claude-opus-4-8" {
		t.Errorf("active model = %q, want anthropic/claude-opus-4-8", m.currentSettingsModel())
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Errorf("right-panel key input opened for anthropic with Settings key — must not")
	}
}

// TestApplyModelPickerSelection_APIKeyMissingOpensKeyForm covers
// the only path that should ever open the right-panel key input:
// a paid API-key provider (openai/anthropic/etc.) where the
// user hasn't stored a key yet. The picker routes to the
// right-panel key form, not the (removed) Settings wizard.
func TestApplyModelPickerSelection_APIKeyMissingOpensKeyForm(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-4o"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("anthropic/claude-opus-4-8")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(anthropic) returned nil; should open right-panel key form")
	}
	sess := m.currentSession()
	if sess == nil {
		t.Fatal("no current session")
	}
	if sess.rightPanel.mode != rpModeKeyInput {
		t.Errorf("right-panel mode = %d, want rpModeKeyInput (%d)", sess.rightPanel.mode, rpModeKeyInput)
	}
	if sess.rightPanel.keyInputProvider != "anthropic" {
		t.Errorf("right-panel key input provider = %q, want anthropic", sess.rightPanel.keyInputProvider)
	}
	if sess.rightPanel.keyInputPending != "anthropic/claude-opus-4-8" {
		t.Errorf("right-panel key input pending = %q, want anthropic/claude-opus-4-8", sess.rightPanel.keyInputPending)
	}
	// And the placeholder must NOT say "API key" for OAuth
	// providers; for missing api-key it should. This catches a
	// regression where the wrong placeholder leaks in.
	if !strings.Contains(sess.rightPanel.keyInput.Placeholder, "anthropic") {
		t.Errorf("placeholder = %q, want to mention anthropic", sess.rightPanel.keyInput.Placeholder)
	}
}

// TestHandleCommandAction_OpenModelPicker_OpensOverlay pins the
// /model slash command wiring. handleCommandAction must call
// ModelPicker.Open so the picker actually appears when the user
// types /model. Before this fix, the action was registered in
// slashmenu.go but had no case in handleCommandAction, so
// /model silently did nothing.
func TestHandleCommandAction_OpenModelPicker_OpensOverlay(t *testing.T) {
	cfg := &cortexconfig.Config{}
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)
	if m.modelPicker.IsVisible() {
		t.Fatal("picker visible at start")
	}
	cmds := m.handleCommandAction("open_model_picker", m.currentSession())
	_ = cmds // returns nil; the actual picker key handling lives in Update
	if !m.modelPicker.IsVisible() {
		t.Error("after open_model_picker, picker should be visible")
	}
}

// TestHandleCommandAction_SelfUpdate_EmitsStatus covers the
// /update slash command. It should at minimum emit a status
// message ("Checking for updates\u2026") so the user sees
// feedback. The actual download runs asynchronously.
func TestHandleCommandAction_SelfUpdate_EmitsStatus(t *testing.T) {
	cfg := &cortexconfig.Config{}
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)
	cmds := m.handleCommandAction("self_update", m.currentSession())
	if len(cmds) == 0 {
		t.Fatal("self_update returned no cmds; should at least emit a status message")
	}
}
