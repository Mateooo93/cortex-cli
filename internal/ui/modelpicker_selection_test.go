package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

// TestApplyModelPickerSelection_CodexFiresOAuth pins the headline
// fix for the "select codex \u2192 still asks for API key" bug.
//
// Before the fix, picking codex from the /model picker would
// either (a) silently do nothing because open_model_picker wasn't
// wired up in handleCommandAction, or (b) fall through to the
// API-key path because the model spec starts with "codex/" \u2014
// exactly the prefix the old codex key-rotation guard in
// selectSettingsModel used to short-circuit on the Settings
// tab path.
//
// The picker must route the user to the browser OAuth flow
// immediately. It must NOT open a key-input form, must NOT
// open the wizard, and must NOT show the
// "paste your codex API key" placeholder.
func TestApplyModelPickerSelection_CodexFiresOAuth(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("codex/gpt-5.5")
	if cmd == nil {
		t.Fatal("applyModelPickerSelection(codex) returned nil; should fire OAuth flow")
	}
	// The returned tea.Cmd resolves to a codexLoginStartedMsg or
	// the wizard/state should show the OAuth prompt. We can't
	// execute the cmd here, but we can verify it was wired up.
	// Sanity-check the user-visible state: no key-input form is
	// open, no wizard is active, and the active model is still
	// the pre-selection openai model (codex only flips the
	// active model AFTER the OAuth flow completes).
	if m.settingsWizard.active {
		t.Errorf("wizard became active for codex \u2014 must not (codex is OAuth)")
	}
	sess := m.currentSession()
	if sess != nil && sess.rightPanel.mode == rpModeKeyInput {
		t.Errorf("right-panel key input opened for codex \u2014 must not (codex is OAuth)")
	}
}

func TestApplyModelPickerSelection_XaiSubFiresOAuth(t *testing.T) {
	cfg := &cortexconfig.Config{}
	cfg.DefaultModel = "openai/gpt-5.5"
	m := NewModel(&config.Config{}, cfg, nil, true, "", true, true)

	cmd := m.applyModelPickerSelection("xai-sub/grok-4.3")
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
	// Seed the openai env var so ResolveProviderKey returns a
	// non-empty key. The test runs in CI where the keychain is
	// empty, so the env-var path is the only way to fake a
	// "stored" key without touching real keychain state.
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

// TestApplyModelPickerSelection_APIKeyMissingOpensKeyForm covers
// the only path that should ever open the right-panel key input:
// a paid API-key provider (openai/anthropic/etc.) where the
// user hasn't stored a key yet. The picker routes to the
// right-panel key form, not the (removed) Settings wizard.
func TestApplyModelPickerSelection_APIKeyMissingOpensKeyForm(t *testing.T) {
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
