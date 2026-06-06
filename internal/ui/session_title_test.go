package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
)

func TestIsFirstUserMessage(t *testing.T) {
	cases := []struct {
		name string
		msgs []ChatMessage
		want bool
	}{
		{
			name: "no messages at all",
			msgs: nil,
			want: true,
		},
		{
			name: "only system messages",
			msgs: []ChatMessage{
				{Type: MsgSystem, Text: "ready"},
				{Type: MsgAssistant, Text: "hi"},
			},
			want: true,
		},
		{
			name: "one user message present",
			msgs: []ChatMessage{
				{Type: MsgAssistant, Text: "ready"},
				{Type: MsgUser, Text: "hello"},
			},
			want: false,
		},
		{
			name: "user message after a tool call still counts as a real user message",
			msgs: []ChatMessage{
				{Type: MsgToolCall, Text: "ls"},
				{Type: MsgUser, Text: "explain that"},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFirstUserMessage(tc.msgs); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCleanAITitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Refactor auth middleware", "Refactor auth middleware"},
		{"  \"Refactor auth\"  ", "Refactor auth"},
		{"Title: Add caching layer", "Add caching layer"},
		{"- Debug flaky test", "Debug flaky test"},
		{"* Build the project", "Build the project"},
		{"Quick fix.", "Quick fix"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		got := cleanAITitle(tc.in)
		if got != tc.want {
			t.Errorf("cleanAITitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDeriveTitleFromMessage(t *testing.T) {
	if got := deriveTitleFromMessage(""); got != "" {
		t.Errorf("empty input should produce empty title, got %q", got)
	}
	if got := deriveTitleFromMessage("   "); got != "" {
		t.Errorf("whitespace should produce empty title, got %q", got)
	}
	if got := deriveTitleFromMessage("/clear"); got != "" {
		t.Errorf("slash commands should be skipped, got %q", got)
	}
	if got := deriveTitleFromMessage("Help me with auth"); got != "Help me with auth" {
		t.Errorf("plain message should be returned, got %q", got)
	}
	// Length capping kicks in.
	longMsg := "this is a long user message that exceeds the cap and should be truncated to fit"
	got := deriveTitleFromMessage(longMsg)
	if len(got) > 48 {
		t.Errorf("expected length <= 48, got %d (%q)", len(got), got)
	}
}

func TestSettingsWizardViewMasksKey(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("https://api.openai.com/v1")
	w := settingsWizard{
		active:   true,
		provider: "openai",
		isCustom: false,
		name:     "openai",
		baseURL:  "https://api.openai.com/v1",
		apiKey:   "sk-supersecret1234",
		field:    wizardFieldBaseURL,
		input:    ti,
	}
	m := &Model{settingsWizard: w}
	view := m.settingsWizardView()
	if view.Provider != "openai" {
		t.Errorf("expected provider openai, got %q", view.Provider)
	}
	if len(view.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(view.Fields))
	}
	// The API key field should be masked (not show the full secret).
	apiKeyField := view.Fields[2]
	if apiKeyField.Value == "sk-supersecret1234" {
		t.Errorf("expected API key to be masked, got full value %q", apiKeyField.Value)
	}
	if !strings.Contains(apiKeyField.Value, "1234") {
		t.Errorf("expected masked API key to keep the last 4 chars for context, got %q", apiKeyField.Value)
	}
	// Base URL should be shown verbatim.
	baseURLField := view.Fields[1]
	if baseURLField.Value != "https://api.openai.com/v1" {
		t.Errorf("expected base URL to be visible, got %q", baseURLField.Value)
	}
}

func TestSettingsWizardViewRevealsKeyWhenFocused(t *testing.T) {
	ti := textinput.New()
	ti.SetValue("sk-supersecret1234")
	ti.EchoMode = textinput.EchoPassword
	w := settingsWizard{
		active:   true,
		provider: "openai",
		field:    wizardFieldAPIKey,
		name:     "openai",
		baseURL:  "https://api.openai.com/v1",
		apiKey:   "sk-supersecret1234",
		input:    ti,
	}
	m := &Model{settingsWizard: w}
	view := m.settingsWizardView()
	apiKeyField := view.Fields[2]
	if !apiKeyField.Focused {
		t.Errorf("expected API key field to be focused")
	}
	if apiKeyField.Value != "sk-supersecret1234" {
		t.Errorf("expected API key to be revealed when focused, got %q", apiKeyField.Value)
	}
}

func TestSettingsWizardEmptyViewWhenInactive(t *testing.T) {
	m := &Model{settingsWizard: settingsWizard{}}
	view := m.settingsWizardView()
	if view.Provider != "" {
		t.Errorf("expected empty view for inactive wizard, got %+v", view)
	}
	if len(view.Fields) != 0 {
		t.Errorf("expected zero fields for inactive wizard, got %d", len(view.Fields))
	}
}

func TestWizardMoveFieldCycles(t *testing.T) {
	w := settingsWizard{
		active:   true,
		provider: "openai",
		field:    wizardFieldName,
		input:    textinput.New(),
	}
	m := &Model{settingsWizard: w}
	// down from name -> baseURL -> apiKey -> back to name (wrap)
	m.wizardMoveField(+1)
	if m.settingsWizard.field != wizardFieldBaseURL {
		t.Errorf("expected baseURL, got %d", m.settingsWizard.field)
	}
	m.wizardMoveField(+1)
	if m.settingsWizard.field != wizardFieldAPIKey {
		t.Errorf("expected apiKey, got %d", m.settingsWizard.field)
	}
	m.wizardMoveField(+1)
	if m.settingsWizard.field != wizardFieldName {
		t.Errorf("expected name (wrap), got %d", m.settingsWizard.field)
	}
	// up wraps the other way
	m.wizardMoveField(-1)
	if m.settingsWizard.field != wizardFieldAPIKey {
		t.Errorf("expected apiKey, got %d", m.settingsWizard.field)
	}
}

func TestWizardIsFieldEditable(t *testing.T) {
	preset := &settingsWizard{isCustom: false}
	if preset.isFieldEditable(wizardFieldName) {
		t.Errorf("preset provider name should not be editable")
	}
	if !preset.isFieldEditable(wizardFieldBaseURL) {
		t.Errorf("base URL should be editable for preset")
	}
	if !preset.isFieldEditable(wizardFieldAPIKey) {
		t.Errorf("API key should be editable for preset")
	}

	custom := &settingsWizard{isCustom: true}
	if !custom.isFieldEditable(wizardFieldName) {
		t.Errorf("custom provider name should be editable")
	}
}

func TestSessionTitleGeneratedMsgAppliesLabel(t *testing.T) {
	m := &Model{}
	sess := &SessionState{persistID: "test-session-42", label: ""}
	m.sessions = []*SessionState{sess}
	next, _ := m.Update(sessionTitleGeneratedMsg{sessionID: "test-session-42", title: "Refactor auth"})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if nm.sessions[0].label != "Refactor auth" {
		t.Errorf("expected label to be set, got %q", nm.sessions[0].label)
	}
}

func TestSessionTitleGeneratedMsgRespectsExistingLabel(t *testing.T) {
	m := &Model{}
	sess := &SessionState{persistID: "test-session-42", label: "User renamed"}
	m.sessions = []*SessionState{sess}
	next, _ := m.Update(sessionTitleGeneratedMsg{sessionID: "test-session-42", title: "Refactor auth"})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if nm.sessions[0].label != "User renamed" {
		t.Errorf("expected existing label to be preserved, got %q", nm.sessions[0].label)
	}
}

func TestSessionTitleGeneratedMsgEmptyTitleNoOp(t *testing.T) {
	m := &Model{}
	sess := &SessionState{persistID: "test-session-42", label: ""}
	m.sessions = []*SessionState{sess}
	next, _ := m.Update(sessionTitleGeneratedMsg{sessionID: "test-session-42", title: ""})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", next)
	}
	if nm.sessions[0].label != "" {
		t.Errorf("expected label to remain empty, got %q", nm.sessions[0].label)
	}
}

func TestSettingsWizardFieldLabel(t *testing.T) {
	w := &settingsWizard{}
	if w.fieldLabel(wizardFieldName) != "Name" {
		t.Errorf("expected Name, got %q", w.fieldLabel(wizardFieldName))
	}
	if w.fieldLabel(wizardFieldBaseURL) != "Base URL" {
		t.Errorf("expected Base URL, got %q", w.fieldLabel(wizardFieldBaseURL))
	}
	if w.fieldLabel(wizardFieldAPIKey) != "API key" {
		t.Errorf("expected API key, got %q", w.fieldLabel(wizardFieldAPIKey))
	}
}

func TestSettingsWizardFieldPlaceholder(t *testing.T) {
	preset := &settingsWizard{isCustom: false}
	if !strings.Contains(preset.fieldPlaceholder(wizardFieldName), "preset") {
		t.Errorf("preset name placeholder should mention preset, got %q", preset.fieldPlaceholder(wizardFieldName))
	}
	custom := &settingsWizard{isCustom: true}
	if strings.Contains(custom.fieldPlaceholder(wizardFieldName), "preset") {
		t.Errorf("custom name placeholder should not mention preset, got %q", custom.fieldPlaceholder(wizardFieldName))
	}
	if preset.fieldPlaceholder(wizardFieldBaseURL) == "" {
		t.Errorf("base URL placeholder should not be empty")
	}
}
