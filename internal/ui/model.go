package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/ultraviolet/screen"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider/codex"
	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

// teaProgram holds the Bubble Tea program reference for event injection via Send().
var teaProgram *tea.Program

// SetProgram stores the tea.Program reference. Call before p.Run().
func SetProgram(p *tea.Program) { teaProgram = p }

// --- Internal message types ---

// sessionEventMsg carries a daemon session event tagged with the daemon session
// ID of the connection that produced it. Messages whose daemonSessionID no
// longer matches the session's current daemonSessionID are silently dropped
// (they came from a superseded connection's goroutine).
type sessionEventMsg struct {
	daemonSessionID string
	event           protocol.SessionEvent
}

// sessionDisconnectedMsg is sent when a session's daemon connection is lost.
type sessionDisconnectedMsg struct {
	daemonSessionID string
}

// codexLoginStartedMsg is fired when the codex OAuth flow begins, so
// the UI can show a status line ("Signing in with ChatGPT…").
type codexLoginStartedMsg struct {
	pendingModel string
	authorizeURL string
}

// codexLoginSuccessMsg is fired when the OAuth flow completes and the
// token is in the keychain. The UI then switches the active model.
type codexLoginSuccessMsg struct {
	pendingModel string
	email        string
	planType     string
}

// codexLoginFailedMsg is fired when OAuth fails (browser can't open,
// user denied, etc.). The UI shows the error in the status bar.
type codexLoginFailedMsg struct {
	err          error
	authorizeURL string // empty if no URL was generated
}

// reconnectSuccessMsg is sent when reconnection succeeds.
// daemonSessionID is the ID of the session we were reconnecting for (the old
// one); client is the newly established connection with its own fresh ID.
type reconnectSuccessMsg struct {
	daemonSessionID string
	client          *daemon.SessionClient
}

// reconnectFailedMsg is sent when reconnection fails.
type reconnectFailedMsg struct {
	daemonSessionID string
}

// resumeFromSleepMsg is sent when the process receives SIGCONT.
type resumeFromSleepMsg struct{}

// StatusMsgKind identifies the visual style of a transient status bar message.
type StatusMsgKind int

const (
	StatusMsgWarning StatusMsgKind = iota
	StatusMsgInfo
	StatusMsgError
)

// StatusMessage is a transient message shown on the second line of the status bar.
type StatusMessage struct {
	Text string
	Kind StatusMsgKind
	gen  int // stale-clear guard
}

type settingsInputMode int

const (
	settingsInputNone settingsInputMode = iota
	settingsInputAPIKey
	settingsInputBaseURL
	settingsInputCustomProviderName
	settingsInputCustomProviderBaseURL
	settingsInputCustomProviderAPIKey
)

// settingsWizardField identifies the focused field inside a provider edit
// wizard. Field order is also the navigation order with up/down arrows.
type settingsWizardField int

const (
	wizardFieldName settingsWizardField = iota
	wizardFieldBaseURL
	wizardFieldAPIKey
)

// settingsWizard holds the state of the full-section provider edit wizard.
// The wizard opens when the user selects a provider in the Providers
// section. It replaces the provider list and presents three editable
// fields (Name, Base URL, API Key). Arrows move between fields; Enter
// commits the current field to disk; Esc commits any in-flight changes
// and closes the wizard.
type settingsWizard struct {
	active   bool
	provider string
	isCustom bool
	field    settingsWizardField

	// Working values. The active field is bound to the text input;
	// the other two are kept in sync with the input as the user types.
	name    string
	baseURL string
	apiKey  string

	// The text input is always focused; arrows switch fields by
	// loading/saving its value into the working-value slots above.
	input textinput.Model
}

type modelsFetchedMsg struct {
	provider string
	baseURL  string
	models   []string
	err      error
}

// clearStatusMsgMsg clears the status bar message when its generation matches.
type clearStatusMsgMsg struct{ gen int }

// startCursorBlinkMsg triggers cursor blink on startup.
type startCursorBlinkMsg struct{}

func waitForResume() tea.Msg {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGCONT)
	<-sigCh
	signal.Stop(sigCh)
	return resumeFromSleepMsg{}
}

// startSessionEventLoop launches a goroutine that reads daemon events for one
// session and injects them into the Bubble Tea loop tagged with the daemon
// session ID captured at launch time. When a session reconnects it gets a new
// daemon session ID, so any in-flight messages from the old goroutine are
// naturally ignored by the handler's ID check — no generation counter needed.
func startSessionEventLoop(client *daemon.SessionClient) tea.Cmd {
	daemonSessionID := client.SessionID()
	return func() tea.Msg {
		if teaProgram == nil {
			return sessionDisconnectedMsg{daemonSessionID: daemonSessionID}
		}
		go func() {
			for {
				event, err := client.ReadEvent()
				if err != nil {
					teaProgram.Send(sessionDisconnectedMsg{daemonSessionID: daemonSessionID})
					return
				}
				teaProgram.Send(sessionEventMsg{daemonSessionID: daemonSessionID, event: event})
			}
		}()
		return nil
	}
}

// attemptReconnect tries to reconnect a session to the daemon.
// targetDaemonSessionID identifies which session this attempt is for; it is
// echoed back in the result message so the handler can match it to the right
// session. Pass an empty string for a session that has never connected — the
// handler will not retry on failure in that case.
func attemptReconnect(socketPath, cwd, configDir, model, authToken string, forceInit, enableWrite, enableDir bool, targetDaemonSessionID string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewClient(socketPath)
		client.SetAuthToken(authToken)
		if !client.Ping() {
			time.Sleep(2 * time.Second)
			return reconnectFailedMsg{daemonSessionID: targetDaemonSessionID}
		}
		session := daemon.NewSessionClient(socketPath)
		session.SetAuthToken(authToken)
		if err := session.Connect(cwd, configDir, model, forceInit, enableWrite, enableDir, false); err != nil {
			time.Sleep(2 * time.Second)
			return reconnectFailedMsg{daemonSessionID: targetDaemonSessionID}
		}
		return reconnectSuccessMsg{daemonSessionID: targetDaemonSessionID, client: session}
	}
}

// connectFork starts a new forked session seeded from forkSessionID at forkTurnIdx.
func connectFork(socketPath, cwd, configDir, model, authToken string, enableWrite, enableDir bool, forkSessionID string, forkTurnIdx int, targetDaemonSessionID string) tea.Cmd {
	return func() tea.Msg {
		client := daemon.NewClient(socketPath)
		client.SetAuthToken(authToken)
		if !client.Ping() {
			time.Sleep(2 * time.Second)
			return reconnectFailedMsg{daemonSessionID: targetDaemonSessionID}
		}
		session := daemon.NewSessionClient(socketPath)
		session.SetAuthToken(authToken)
		if err := session.ConnectFork(cwd, configDir, model, false, enableWrite, enableDir, false, forkSessionID, forkTurnIdx); err != nil {
			time.Sleep(2 * time.Second)
			return reconnectFailedMsg{daemonSessionID: targetDaemonSessionID}
		}
		return reconnectSuccessMsg{daemonSessionID: targetDaemonSessionID, client: session}
	}
}

// findSessionByDaemonID returns the index and pointer of the session with the
// given daemon session ID, or (-1, nil) if not found.
func (m *Model) findSessionByDaemonID(id string) (int, *SessionState) {
	for i, s := range m.sessions {
		if s.daemonSessionID == id {
			return i, s
		}
	}
	return -1, nil
}

// AppState represents the current state of the application.
type AppState int

const (
	StateWaitingForInput AppState = iota
	StateStreaming
	StateToolExecuting
	StateConfirmPending
	StatePlanReview
	StatePlanExecuting
	StateUserQuestion
	StateQuitConfirm
	StateTrimConfirm
	StateSessionCloseConfirm
)

// pendingMsg holds a user message submitted while the agent was
// streaming. The message is sent on the next event.agent_done, so
// the user can either:
//   - Tab (no cancel) to queue for after the current turn
//     finishes naturally, or
//   - Enter (delayed cancel) to interrupt the current turn
//     after the in-flight edit completes, then send this
//     message immediately.
//
// The Queued field carries a short human label for the input
// placeholder so the user can see *what* is queued and which
// cancel mode was used.
type pendingMsg struct {
	text        string
	attachments []protocol.Attachment
	// Queued is true when the message was queued (Tab) without a
	// cancel. false when the message was queued with a delayed
	// cancel (Enter).
	Queued bool
}

// pendingPlanAction holds a plan action submitted while disconnected.
type pendingPlanAction struct {
	action string
	text   string
}

// Model is the root Bubble Tea model.
type Model struct {
	width, height int

	// Two visible tabs: Sessions list and Chat display.
	activeTab TabKind

	// All active sessions. Each accumulates messages independently.
	sessions        []*SessionState
	selectedSession int // index into sessions; which session the Chat tab shows

	// Global overlay dialog state (quit confirm, session close confirm).
	// Normal operation = StateWaitingForInput (no overlay).
	state                AppState
	quitSelected         int
	sessionCloseIdx      int
	sessionCloseSelected int

	// Sessions tab UI
	sessionsInput    textinput.Model
	sessionsSelected int

	// Settings tab UI
	settingsActiveSection    int    // 0=models, 1=providers/API keys, 2=other settings
	settingsProviderSel      int    // row in ProvidersFromConfig (Model section, column 0)
	settingsModelSel         int    // row in ModelsForProviderFromConfig(selected provider) (Model section, column 1)
	settingsModelColumn      int    // 0 = provider column focused, 1 = model column focused
	settingsModelPending     string // model spec awaiting an API key
	settingsKeySel           int
	settingsKeys             []ProviderSettingsView
	settingsOtherSel         int
	settingsKeyInputProvider string
	settingsKeyInputLabel    string
	settingsKeyInput         textinput.Model
	settingsInKeyInput       bool
	settingsInputMode        settingsInputMode
	settingsCustomProvider   string
	settingsCustomBaseURL    string
	settingsInspectProvider  string
	settingsInspectField     int
	settingsWizard           settingsWizard

	// Shared rendering
	mdRenderer     *MarkdownRenderer
	commandPalette CommandPalette

	// Tab alert blink (Chat tab label pulses when a session needs attention)
	tabAlertActive   bool
	tabAlertBlinkOn  bool
	tabAlertBlinkGen int

	// Transient status bar message (second line)
	statusMsg StatusMessage

	// Connection parameters (for reconnect / new sessions)
	socketPath                     string
	cwd                            string
	authToken                      string
	forceInit                      bool
	enableAutomaticWritePermission bool
	enableAutomaticDirectoryAccess bool

	// Global settings
	hasDarkBG      bool
	styles         Styles
	kittySupported bool
	cfg            *config.Config
	cortexCfg      *cortexconfig.Config
	testMode       bool
}

// currentSession returns the selected session, or nil if there is none.
func (m *Model) currentSession() *SessionState {
	if m.selectedSession < 0 || m.selectedSession >= len(m.sessions) {
		return nil
	}
	return m.sessions[m.selectedSession]
}

// refreshSettingsKeys rebuilds the Settings provider/API rows from Cortex config.
func (m *Model) refreshSettingsKeys() {
	if m.cortexCfg != nil {
		m.cortexCfg.EnsureProviderPresets()
	}
	m.settingsKeys = ProviderSettingsRows(m.cortexCfg)
	if m.settingsKeySel < 0 {
		m.settingsKeySel = 0
	}
	if len(m.settingsKeys) == 0 {
		m.settingsKeySel = 0
		return
	}
	if m.settingsKeySel >= len(m.settingsKeys) {
		m.settingsKeySel = len(m.settingsKeys) - 1
	}
}

func (m *Model) settingsProviders() []ProviderInfo {
	return ProvidersFromConfig(m.cortexCfg)
}

func (m *Model) selectedSettingsProviderName() string {
	providers := m.settingsProviders()
	if len(providers) == 0 {
		return ""
	}
	if m.settingsProviderSel < 0 {
		m.settingsProviderSel = 0
	}
	if m.settingsProviderSel >= len(providers) {
		m.settingsProviderSel = len(providers) - 1
	}
	return providers[m.settingsProviderSel].Name
}

func (m *Model) selectedSettingsModels() []ModelInfo {
	return ModelsForProviderFromConfig(m.selectedSettingsProviderName(), m.cortexCfg)
}

func (m *Model) currentSettingsModel() string {
	if m.cortexCfg != nil && m.cortexCfg.DefaultModel != "" {
		return m.cortexCfg.DefaultModel
	}
	if m.cfg != nil && m.cfg.Model != "" {
		return m.cfg.Model
	}
	if sess := m.currentSession(); sess != nil && sess.modelName != "" {
		return sess.modelName
	}
	return ""
}

func (m *Model) displayNameForModelSpec(spec string) string {
	if spec == "" {
		return ""
	}
	if m.cortexCfg != nil {
		if key, mc, err := m.cortexCfg.GetModel(spec); err == nil && mc != nil {
			if key == spec {
				return cortexconfig.ModelSpec(mc.Provider, mc.Model)
			}
			return key
		}
	}
	return spec
}

func (m *Model) setActiveModelSpec(spec string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return
	}
	if m.cfg != nil {
		m.cfg.Model = spec
	}
	if m.cortexCfg != nil {
		m.cortexCfg.DefaultModel = spec
	}
	if sess := m.currentSession(); sess != nil {
		sess.modelName = spec
	}
}

func (m *Model) activeModelForNewSession() string {
	if m.cortexCfg != nil && m.cortexCfg.DefaultModel != "" {
		return m.cortexCfg.DefaultModel
	}
	if m.cfg != nil && m.cfg.Model != "" {
		return m.cfg.Model
	}
	return m.currentSettingsModel()
}

func (m *Model) canonicalSettingsModel(spec string) string {
	return normalizeSettingsModelSpec(spec, m.cortexCfg)
}

func (m *Model) openSettingsTab() {
	m.activeTab = TabKindSettings
	m.refreshSettingsKeys()
	m.settingsInKeyInput = false
	m.settingsInputMode = settingsInputNone
	m.settingsModelPending = ""
	m.settingsKeyInputLabel = ""
	m.settingsCustomProvider = ""
	m.settingsCustomBaseURL = ""
	m.settingsInspectProvider = ""
	m.settingsWizard = settingsWizard{}
	providerSel, modelSel := locateActiveModelFromConfig(m.currentSettingsModel(), m.cortexCfg)
	m.settingsProviderSel = providerSel
	m.settingsModelSel = modelSel
	if m.settingsModelColumn != 1 {
		m.settingsModelColumn = 1
	}
}

func (m *Model) findSettingsKeyIndex(provider string) int {
	for i, pk := range m.settingsKeys {
		if pk.Provider == provider {
			return i
		}
	}
	return -1
}

func (m *Model) settingsInspectView() SettingsInspectView {
	if m.settingsInspectProvider == "" {
		return SettingsInspectView{}
	}
	idx := m.findSettingsKeyIndex(m.settingsInspectProvider)
	if idx < 0 {
		return SettingsInspectView{}
	}
	pk := m.settingsKeys[idx]
	return SettingsInspectView{
		Provider:    pk.Provider,
		DisplayName: pk.DisplayName,
		BaseURL:     pk.BaseURL,
		HasAPIKey:   pk.KeyPrefix != "",
		KeyPrefix:   pk.KeyPrefix,
		Field:       m.settingsInspectField,
		NeedsAPIKey: pk.NeedsAPIKey,
		EnvVar:      pk.EnvVar,
	}
}

// settingsWizardView projects the live wizard state into a view-model
// that the renderer can consume without touching the text input or
// cortexCfg. The API key is masked unless the field is currently
// focused (so the user can see what they have typed).
func (m *Model) settingsWizardView() SettingsWizardView {
	w := m.settingsWizard
	if !w.active {
		return SettingsWizardView{}
	}
	maskKey := func(v string) string {
		if v == "" {
			return ""
		}
		if len(v) <= 4 {
			return strings.Repeat("•", len(v))
		}
		return strings.Repeat("•", len(v)-4) + v[len(v)-4:]
	}
	makeField := func(field settingsWizardField, value string) SettingsWizardFieldView {
		fv := SettingsWizardFieldView{
			Label:       w.fieldLabel(field),
			Placeholder: w.fieldPlaceholder(field),
			Focused:     w.field == field,
			Editable:    w.isFieldEditable(field),
		}
		if field == wizardFieldAPIKey {
			if w.field == field {
				fv.Value = value
			} else {
				fv.Value = maskKey(value)
			}
		} else {
			fv.Value = value
		}
		if !fv.Editable {
			fv.Hint = "preset — read-only"
		}
		return fv
	}
	view := SettingsWizardView{
		Provider:    w.provider,
		DisplayName: w.name,
		IsCustom:    w.isCustom,
		Field:       w.field,
		InputView:   w.input.View(),
		Fields: []SettingsWizardFieldView{
			makeField(wizardFieldName, w.name),
			makeField(wizardFieldBaseURL, w.baseURL),
			makeField(wizardFieldAPIKey, w.apiKey),
		},
	}
	return view
}

func (m *Model) openSettingsTextInput(mode settingsInputMode, provider, label, placeholder, value string, password bool) {
	ti := textinput.New()
	ti.Placeholder = placeholder
	if password {
		ti.EchoMode = textinput.EchoPassword
	}
	ti.SetValue(value)
	ti.Focus()
	m.settingsKeyInput = ti
	m.settingsKeyInputProvider = provider
	m.settingsKeyInputLabel = label
	m.settingsInputMode = mode
	m.settingsInKeyInput = true
}

// openSettingsWizard opens the full-section provider edit wizard for the
// given provider. The wizard replaces the provider list inside the
// Providers section; the rest of the Settings tab is hidden. The wizard
// initialises its three working values from the current cortexCfg (or
// the provider's defaults when unset) and binds the first editable
// field to the text input.
func (m *Model) openSettingsWizard(providerName string) {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	if providerName == "" {
		return
	}
	w := settingsWizard{
		active:   true,
		provider: providerName,
	}
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok {
			w.name = pc.Provider
			if pc.Provider == "" {
				w.name = providerName
			}
			w.isCustom = cortexconfig.IsCustomProvider(providerName)
		}
	}
	if w.name == "" {
		w.name = providerName
	}
	w.baseURL = m.providerBaseURL(providerName)
	w.apiKey = m.providerAPIKey(providerName)
	// Pre-select the most useful field for a typical edit: the API key
	// when the provider needs one and none is set, otherwise base URL.
	if w.apiKey == "" && cortexconfig.ProviderNeedsAPIKey(providerName) {
		w.field = wizardFieldAPIKey
	} else {
		w.field = wizardFieldBaseURL
	}
	w.input = m.newWizardFieldInput(w.field)
	m.settingsWizard = w
	m.settingsInspectProvider = ""
	m.settingsInKeyInput = false
	m.settingsInputMode = settingsInputNone
}

// closeSettingsWizard commits any in-flight changes, persists the
// provider config, refreshes derived UI state, and closes the wizard.
func (m *Model) closeSettingsWizard() tea.Cmd {
	w := m.settingsWizard
	if !w.active {
		return nil
	}
	m.captureWizardFieldValue()
	m.settingsWizard = settingsWizard{}
	m.refreshSettingsKeys()
	providerName := w.provider
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok {
			if w.isCustom && w.name != "" && cortexconfig.NormalizeProviderName(w.name) != providerName {
				// Custom-provider rename: rotate the config key.
				newName := cortexconfig.NormalizeProviderName(w.name)
				if newName != "" {
					updated := pc
					updated.Provider = newName
					delete(m.cortexCfg.Models, providerName)
					m.cortexCfg.Models[newName] = updated
					_ = cortexconfig.Save(m.cortexCfg)
				}
			}
		}
	}
	return m.fetchModelsForProvider(providerName)
}

// wizardFieldInput creates a fresh text input bound to the given wizard
// field. The input carries the current working value of the field and
// password-echoes the API key.
func (m *Model) newWizardFieldInput(field settingsWizardField) textinput.Model {
	w := &m.settingsWizard
	ti := textinput.New()
	ti.SetValue(w.fieldValue(field))
	if field == wizardFieldAPIKey {
		ti.EchoMode = textinput.EchoPassword
		ti.Placeholder = "Paste API key (leave empty to clear)"
	} else {
		ti.Placeholder = w.fieldPlaceholder(field)
	}
	ti.Focus()
	return ti
}

// fieldValue returns the working value of a wizard field.
func (w *settingsWizard) fieldValue(field settingsWizardField) string {
	switch field {
	case wizardFieldName:
		return w.name
	case wizardFieldBaseURL:
		return w.baseURL
	case wizardFieldAPIKey:
		return w.apiKey
	}
	return ""
}

// setFieldValue updates the working value of a wizard field.
func (w *settingsWizard) setFieldValue(field settingsWizardField, value string) {
	switch field {
	case wizardFieldName:
		w.name = value
	case wizardFieldBaseURL:
		w.baseURL = value
	case wizardFieldAPIKey:
		w.apiKey = value
	}
}

// fieldPlaceholder returns the placeholder text for a wizard field.
func (w *settingsWizard) fieldPlaceholder(field settingsWizardField) string {
	switch field {
	case wizardFieldName:
		if w.isCustom {
			return "e.g. groq, together, local-ai"
		}
		return "Provider name (preset — read-only)"
	case wizardFieldBaseURL:
		return "https://example.com/v1"
	}
	return ""
}

// fieldLabel returns the human-readable label for a wizard field.
func (w *settingsWizard) fieldLabel(field settingsWizardField) string {
	switch field {
	case wizardFieldName:
		return "Name"
	case wizardFieldBaseURL:
		return "Base URL"
	case wizardFieldAPIKey:
		return "API key"
	}
	return ""
}

// isFieldEditable reports whether the wizard field accepts edits.
func (w *settingsWizard) isFieldEditable(field settingsWizardField) bool {
	if field == wizardFieldName {
		return w.isCustom
	}
	return true
}

// captureWizardFieldValue writes the current text input value back into
// the wizard's working-value slot for the active field.
func (m *Model) captureWizardFieldValue() {
	w := &m.settingsWizard
	if !w.active {
		return
	}
	w.setFieldValue(w.field, w.input.Value())
}

// wizardMoveField switches the wizard's active field by delta (typically
// +1 for down, -1 for up). The current text input's value is captured
// into the outgoing field, then the input is rebound to the new field's
// working value.
func (m *Model) wizardMoveField(delta int) tea.Cmd {
	w := &m.settingsWizard
	if !w.active {
		return nil
	}
	m.captureWizardFieldValue()
	fields := []settingsWizardField{wizardFieldName, wizardFieldBaseURL, wizardFieldAPIKey}
	idx := 0
	for i, f := range fields {
		if f == w.field {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(fields)) % len(fields)
	w.field = fields[idx]
	w.input = m.newWizardFieldInput(w.field)
	return nil
}

// wizardCommitCurrent writes the active field's value to cortexCfg and
// persists it to disk. The wizard stays open and the field stays
// selected so the user can keep editing.
func (m *Model) wizardCommitCurrent() tea.Cmd {
	w := &m.settingsWizard
	if !w.active {
		return nil
	}
	m.captureWizardFieldValue()
	providerName := w.provider
	if m.cortexCfg == nil {
		return nil
	}
	var status string
	switch w.field {
	case wizardFieldName:
		if w.isCustom {
			newName := cortexconfig.NormalizeProviderName(w.name)
			if newName == "" {
				return m.emitStatusMsg("Provider name cannot be empty", StatusMsgError)
			}
			if newName != providerName {
				if pc, ok := m.cortexCfg.Models[providerName]; ok {
					pc.Provider = newName
					delete(m.cortexCfg.Models, providerName)
					m.cortexCfg.Models[newName] = pc
					_ = cortexconfig.Save(m.cortexCfg)
					w.provider = newName
					w.isCustom = true
					status = "Provider renamed to " + newName
				}
			}
		}
	case wizardFieldBaseURL:
		val := strings.TrimSpace(w.baseURL)
		if val == "" {
			return m.emitStatusMsg("Base URL cannot be empty", StatusMsgError)
		}
		m.cortexCfg.SetProviderBaseURL(providerName, val)
		_ = cortexconfig.Save(m.cortexCfg)
		status = "Base URL saved for " + providerName
	case wizardFieldAPIKey:
		m.cortexCfg.SetProviderAPIKey(providerName, strings.TrimSpace(w.apiKey))
		_ = cortexconfig.Save(m.cortexCfg)
		if w.apiKey == "" {
			status = "API key cleared for " + providerName
		} else {
			status = "API key saved for " + providerName
		}
	}
	m.refreshSettingsKeys()
	if w.field == wizardFieldAPIKey || w.field == wizardFieldBaseURL {
		return tea.Batch(m.emitStatusMsg(status, StatusMsgInfo), m.fetchModelsForProvider(providerName))
	}
	return m.emitStatusMsg(status, StatusMsgInfo)
}

func (m *Model) settingsProviderHasKey(providerName string) bool {
	if !cortexconfig.ProviderNeedsAPIKey(providerName) {
		return true
	}
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok && pc.APIKey != "" {
			return true
		}
	}
	if envVar := cortexconfig.ProviderEnvVar(providerName); envVar != "" && os.Getenv(envVar) != "" {
		return true
	}
	return false
}

func (m *Model) providerAPIKey(providerName string) string {
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok && pc.APIKey != "" {
			return pc.APIKey
		}
	}
	if envVar := cortexconfig.ProviderEnvVar(providerName); envVar != "" {
		return os.Getenv(envVar)
	}
	return ""
}

func (m *Model) providerBaseURL(providerName string) string {
	if m.cortexCfg != nil {
		if pc, ok := m.cortexCfg.ProviderConfig(providerName); ok && pc.BaseURL != "" {
			return pc.BaseURL
		}
	}
	return cortexconfig.DefaultBaseURL(providerName)
}

func (m *Model) selectSettingsModel(mod ModelInfo) tea.Cmd {
	spec := normalizeSettingsModelSpec(mod.Spec, m.cortexCfg)
	if spec == "" {
		return m.emitStatusMsg("Model selection is empty", StatusMsgError)
	}
	modelID := strings.TrimSpace(mod.DisplayName)
	if _, parsedModel, ok := cortexconfig.SplitModelSpec(spec); ok {
		modelID = parsedModel
	}
	if m.cortexCfg != nil {
		if _, _, err := m.cortexCfg.GetModel(spec); err != nil {
			if ensured := m.cortexCfg.EnsureProviderModel(mod.Provider, modelID); ensured != "" {
				spec = ensured
			}
		}
		m.cortexCfg.DefaultModel = spec
		_ = cortexconfig.Save(m.cortexCfg)
	}
	m.setActiveModelSpec(spec)
	if settSess := m.currentSession(); settSess != nil && settSess.client != nil {
		_ = settSess.client.SendSetModel(spec)
	}
	m.refreshSettingsKeys()
	m.settingsProviderSel, m.settingsModelSel = locateActiveModelFromConfig(spec, m.cortexCfg)
	return m.emitStatusMsg("Model set to "+mod.DisplayName, StatusMsgInfo)
}

func (m *Model) fetchModelsForProvider(providerName string) tea.Cmd {
	providerName = cortexconfig.NormalizeProviderName(providerName)
	baseURL := m.providerBaseURL(providerName)
	apiKey := m.providerAPIKey(providerName)
	candidates := m.modelRefreshBaseURLCandidates(providerName, baseURL)
	return func() tea.Msg {
		models, usedBaseURL, err := llmprovider.FetchModelsFromCandidates(context.Background(), apiKey, candidates...)
		return modelsFetchedMsg{provider: providerName, baseURL: usedBaseURL, models: models, err: err}
	}
}

// startCodexLoginCmd kicks off the codex OAuth flow in the background.
// The returned tea.Cmd resolves to a codexLoginStartedMsg, which the
// Update() loop turns into either a success or failure message after
// the browser round-trip. The model.go handler is responsible for
// applying the resulting token to the active model.
func (m *Model) startCodexLoginCmd(pendingModel string) tea.Cmd {
	return func() tea.Msg {
		// Give the user a 5-minute window to complete the OAuth flow.
		// Plenty for the typical 5–10s ChatGPT sign-in; long enough
		// that an SSO detour or two doesn't time out.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		res, err := codex.Login(ctx)
		if err == nil && res != nil && res.Token != nil {
			if saveErr := codex.Save(res.Token); saveErr != nil {
				return codexLoginFailedMsg{err: fmt.Errorf("codex: save token: %w", saveErr)}
			}
			return codexLoginSuccessMsg{
				pendingModel: pendingModel,
				email:        res.Token.Email,
				planType:     res.Token.PlanType,
			}
		}
		authURL := ""
		if res != nil {
			authURL = res.AuthorizeURL
		}
		return codexLoginFailedMsg{err: err, authorizeURL: authURL}
	}
}

// handleCodexLoginSuccess applies the freshly saved OAuth token by
// switching the active model to pendingModel. Mirrors the API-key
// "key stored" path.
func (m *Model) handleCodexLoginSuccess(pendingModel, email, planType string) tea.Cmd {
	sess := m.currentSession()
	if pendingModel != "" {
		m.setActiveModelSpec(pendingModel)
		if m.cortexCfg != nil {
			m.cortexCfg.DefaultModel = pendingModel
			_ = cortexconfig.Save(m.cortexCfg)
		}
		if sess != nil && sess.client != nil {
			_ = sess.client.SendSetModel(pendingModel)
		}
		m.refreshSettingsKeys()
		m.settingsProviderSel, m.settingsModelSel = locateActiveModelFromConfig(pendingModel, m.cortexCfg)
	}
	who := email
	if who == "" {
		who = "ChatGPT account"
	}
	if planType != "" {
		who = who + " (" + planType + ")"
	}
	return m.emitStatusMsg("Signed in to "+who, StatusMsgInfo)
}

// handleCodexLoginFailed surfaces the failure in the status bar.
// If the browser couldn't be opened, include the authorize URL so the
// user can copy it from the status message and paste into a browser
// manually (e.g. on a headless server).
func (m *Model) handleCodexLoginFailed(err error, authorizeURL string) tea.Cmd {
	msg := "ChatGPT sign-in failed: " + err.Error()
	if authorizeURL != "" {
		msg += " — open " + authorizeURL + " manually"
	}
	return m.emitStatusMsg(msg, StatusMsgError)
}

func (m *Model) modelRefreshBaseURLCandidates(providerName, baseURL string) []string {
	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmedBaseURL == "" {
		return nil
	}

	var candidates []string
	seen := map[string]bool{}
	appendCandidate := func(candidate string) {
		candidate = strings.TrimRight(strings.TrimSpace(candidate), "/")
		if candidate == "" {
			return
		}
		key := strings.ToLower(candidate)
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}
	appendScopedCandidate := func(scope string) {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == "" {
			return
		}
		if strings.HasSuffix(strings.ToLower(trimmedBaseURL), "/"+scope) {
			return
		}
		appendCandidate(trimmedBaseURL + "/" + scope)
	}

	appendCandidate(trimmedBaseURL)
	for _, scope := range m.providerModelScopes(providerName) {
		appendScopedCandidate(scope)
	}
	if providerName == "opengateway" || strings.Contains(strings.ToLower(trimmedBaseURL), "opengateway") {
		for _, scope := range []string{"xiaomi", "google", "minimax", "qwen", "nvidia"} {
			appendScopedCandidate(scope)
		}
	} else if cortexconfig.DefaultBaseURL(providerName) == "" {
		appendScopedCandidate(providerName)
	}
	return candidates
}

func (m *Model) providerModelScopes(providerName string) []string {
	if m.cortexCfg == nil {
		return nil
	}
	providerName = cortexconfig.NormalizeProviderName(providerName)
	seen := map[string]bool{}
	var scopes []string
	for _, mc := range m.cortexCfg.Models {
		if cortexconfig.NormalizeProviderName(mc.Provider) != providerName {
			continue
		}
		prefix, _, ok := cortexconfig.SplitModelSpec(mc.Model)
		if !ok {
			continue
		}
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if prefix == "" || prefix == providerName || seen[prefix] {
			continue
		}
		seen[prefix] = true
		scopes = append(scopes, prefix)
	}
	return scopes
}

func (m *Model) openAPIKeyInput(providerName string) {
	m.openSettingsTextInput(settingsInputAPIKey, providerName, "Provider: "+providerName+" API key", "Paste your "+providerName+" API key...", "", true)
}

func normalizedTheme(theme string) string {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "dark", "light", "auto":
		return strings.ToLower(strings.TrimSpace(theme))
	default:
		return "auto"
	}
}

func nextTheme(theme string) string {
	switch normalizedTheme(theme) {
	case "auto":
		return "dark"
	case "dark":
		return "light"
	default:
		return "auto"
	}
}

func normalizedReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "auto", "low", "medium", "high", "xhigh", "minimal", "none":
		return strings.ToLower(strings.TrimSpace(effort))
	default:
		return "auto"
	}
}

func nextReasoningEffort(effort string) string {
	switch normalizedReasoningEffort(effort) {
	case "auto":
		return "low"
	case "low":
		return "medium"
	case "medium":
		return "high"
	case "high":
		return "xhigh"
	case "xhigh":
		return "minimal"
	case "minimal":
		return "none"
	default:
		return "auto"
	}
}

func (m *Model) configuredTheme() string {
	if m.cortexCfg == nil {
		return "auto"
	}
	return normalizedTheme(m.cortexCfg.Theme)
}

func (m *Model) applyConfiguredTheme() {
	theme := m.configuredTheme()
	switch theme {
	case "dark":
		m.hasDarkBG = true
	case "light":
		m.hasDarkBG = false
	}
	m.styles = NewStyles(m.hasDarkBG)
	width := 80
	if m.mdRenderer != nil {
		width = m.mdRenderer.width
	}
	m.mdRenderer = NewMarkdownRenderer(width, m.hasDarkBG, m.styles.CodeBoxBorderStyle)
}

func (m *Model) currentReasoningEffort() string {
	if m.cortexCfg == nil {
		return "auto"
	}
	_, mc, err := m.cortexCfg.GetModel(m.currentSettingsModel())
	if err != nil || mc == nil {
		return "auto"
	}
	return normalizedReasoningEffort(mc.ReasoningEffort)
}

func (m *Model) setActiveReasoningEffort(effort string) {
	if m.cortexCfg == nil {
		return
	}
	key, mc, err := m.cortexCfg.GetModel(m.currentSettingsModel())
	if err != nil || mc == nil || key == "" {
		return
	}
	updated := *mc
	updated.ReasoningEffort = normalizedReasoningEffort(effort)
	m.cortexCfg.Models[key] = updated
	_ = cortexconfig.Save(m.cortexCfg)
}

func (m *Model) configuredStreaming() bool {
	if m.cortexCfg == nil {
		return true
	}
	return m.cortexCfg.Streaming
}

func (m *Model) configuredShowUsage() bool {
	if m.cortexCfg == nil {
		return true
	}
	return m.cortexCfg.ShowUsage
}

func (m *Model) setConfiguredTheme(theme string) {
	if m.cortexCfg == nil {
		return
	}
	m.cortexCfg.Theme = normalizedTheme(theme)
	_ = cortexconfig.Save(m.cortexCfg)
	m.applyConfiguredTheme()
}

func (m *Model) setConfiguredStreaming(v bool) {
	if m.cortexCfg == nil {
		return
	}
	m.cortexCfg.Streaming = v
	_ = cortexconfig.Save(m.cortexCfg)
}

func (m *Model) setConfiguredShowUsage(v bool) {
	if m.cortexCfg == nil {
		return
	}
	m.cortexCfg.ShowUsage = v
	_ = cortexconfig.Save(m.cortexCfg)
}

// settingsOtherOptionCount matches the row count rendered in renderSettingsView
// for the "Other Settings" section. Keep in sync with tabs.go.
const settingsOtherOptionCount = 5

func (m *Model) setAllSessionsShowThinking(show bool) {
	for _, sess := range m.sessions {
		if sess == nil {
			continue
		}
		sess.showThinking = show
		if show && sess.thinkingBuf != "" {
			sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
		} else {
			sess.thinkingRendered = ""
		}
	}
	_ = config.SetShowThinking(show)
}

func NewModel(cfg *config.Config, cortexCfg *cortexconfig.Config, client *daemon.SessionClient, testMode bool, authToken string, enableWrite, enableDir bool) Model {
	if cortexCfg == nil {
		cortexCfg = cortexconfig.Default()
	}
	cortexCfg.EnsureProviderPresets()
	cfg.Model = cortexCfg.DefaultModel
	initialSession := newSessionState(cfg, client)

	m := Model{
		state:                          StateWaitingForInput,
		activeTab:                      TabKindChat,
		sessions:                       []*SessionState{initialSession},
		selectedSession:                0,
		sessionsInput:                  newSessionsInput(),
		commandPalette:                 NewCommandPalette(),
		hasDarkBG:                      true,
		styles:                         NewStyles(true),
		mdRenderer:                     NewMarkdownRenderer(80, true, NewStyles(true).CodeBoxBorderStyle),
		cfg:                            cfg,
		cortexCfg:                      cortexCfg,
		socketPath:                     cfg.SocketPath,
		cwd:                            cfg.CWD,
		forceInit:                      cfg.ForceInit,
		authToken:                      authToken,
		enableAutomaticWritePermission: enableWrite,
		enableAutomaticDirectoryAccess: enableDir,
		testMode:                       testMode,
	}

	// Restore previously-saved sessions so the user can see and reopen
	// them from the Sessions tab after a restart. We always keep the
	// freshly-connected session at index 0; saved placeholders are
	// appended after it.
	if !testMode {
		saved := loadSavedSessions()
		m.restoreSavedSessions(saved, initialSession)
		// Persist the merged list so the live session's stable ID
		// is recorded for the next launch.
		m.persistSessions()
	}

	if testMode {
		m.fillTestData()
	}
	m.applyConfiguredTheme()

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.testMode {
		return nil
	}
	var cmds []tea.Cmd
	cmds = append(cmds, func() tea.Msg { return startCursorBlinkMsg{} })
	if sess := m.currentSession(); sess != nil && sess.client != nil {
		cmds = append(cmds, startSessionEventLoop(sess.client))
	}
	cmds = append(cmds, waitForResume, tea.RequestBackgroundColor)
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		sess := m.currentSession()
		if sess != nil {
			sess.input.SetWidth(m.width - 4)
			sess.questionPanel.SetWidth(m.width)
		}
		m.sessionsInput.SetWidth(m.width - 6)
		m.updateChatWidth()
		return m, nil

	case tea.KeyPressMsg:
		// --- Global quit confirm overlay ---
		if msg.String() == "ctrl+c" || msg.String() == "ctrl+d" {
			if m.state == StateQuitConfirm {
				sess := m.currentSession()
				if sess != nil && sess.client != nil {
					sess.client.SendCancel()
					sess.client.SendClose()
				}
				// Flush the latest chat scrollback to disk so the
				// user does not lose unsaved messages on quit.
				m.persistSessions()
				return m, tea.Quit
			}
			m.state = StateQuitConfirm
			m.quitSelected = 0
			return m, nil
		}

		// --- Quit / SessionClose / Trim dialogs intercept all keys ---
		if m.state == StateQuitConfirm || m.state == StateSessionCloseConfirm {
			return m.handleDialogKey(msg)
		}
		sess := m.currentSession()
		if sess != nil && sess.agentState == StateTrimConfirm {
			return m.handleTrimKey(msg)
		}

		// --- History panel (Chat tab only) ---
		if m.activeTab == TabKindChat && sess != nil && sess.historyPanel.IsVisible() {
			switch msg.String() {
			case "up", "k":
				sess.historyPanel.MoveUp()
			case "down", "j":
				sess.historyPanel.MoveDown(len(sess.history.entries))
			case "enter":
				if sess.historyPanel.selected >= 0 && sess.historyPanel.selected < len(sess.history.entries) {
					sess.input.Reset()
					sess.input.InsertString(sess.history.entries[sess.historyPanel.selected])
					sess.input.SetHeight(m.visualLineCount())
				}
				sess.historyPanel.Close()
			case "esc":
				sess.historyPanel.Close()
			default:
				sess.historyPanel.Close()
			}
			return m, nil
		}

		// --- Right panel (Chat tab only) ---
		if m.activeTab == TabKindChat && sess != nil && sess.rightPanel.IsVisible() && sess.focus == FocusRightPanel {
			if msg.String() == "tab" {
				sess.focus = FocusEditor
				sess.input.Focus()
				return m, nil
			}
			action, payload := sess.rightPanel.HandleKey(msg)
			switch action {
			case rpActionClose:
				sess.rightPanel.Close()
				m.updateChatWidth()
				sess.input.Focus()
				sess.focus = FocusEditor
			case rpActionModelSelected:
				m.setActiveModelSpec(payload)
				if m.cortexCfg != nil {
					m.cortexCfg.DefaultModel = payload
					_ = cortexconfig.Save(m.cortexCfg)
				}
				if sess.client != nil {
					_ = sess.client.SendSetModel(payload)
				}
				sess.rightPanel.Close()
				m.updateChatWidth()
				sess.input.Focus()
				sess.focus = FocusEditor
			case rpActionNeedKey:
				parts := strings.SplitN(payload, ":", 2)
				if len(parts) == 2 {
					sess.rightPanel.OpenKeyInput(parts[0], parts[1], m.height)
				}
			case rpActionKeyStored:
				parts := strings.SplitN(payload, ":", 2)
				if len(parts) == 2 {
					provider, key := parts[0], parts[1]
					_ = config.StoreProviderKey(provider, key)
					pendingModel := sess.rightPanel.keyInputPending
					if pendingModel != "" {
						m.setActiveModelSpec(pendingModel)
						if m.cortexCfg != nil {
							m.cortexCfg.DefaultModel = pendingModel
							_ = cortexconfig.Save(m.cortexCfg)
						}
						if sess.client != nil {
							_ = sess.client.SendSetModel(pendingModel)
						}
						sess.rightPanel.Close()
						m.updateChatWidth()
						sess.input.Focus()
						sess.focus = FocusEditor
					} else {
						if sess.client != nil && sess.modelName != "" {
							_ = sess.client.SendSetModel(sess.modelName)
						}
						sess.rightPanel.OpenKeyManager(m.height)
						sess.focus = FocusRightPanel
						sess.input.Blur()
					}
				}
			case rpActionKeyDeleted:
				_ = config.DeleteProviderKey(payload)
				sess.rightPanel.OpenKeyManager(m.height)
				sess.focus = FocusRightPanel
				sess.input.Blur()
			case rpActionCodexSignIn:
				// Close the panel and run the OAuth flow in a
				// goroutine. The user will see a brief "Signing in…"
				// message, their browser opens, they sign in, and we
				// fire a tea.Cmd when done so the model can switch
				// back to the chat without blocking the UI.
				pendingModel := payload
				sess.rightPanel.Close()
				m.updateChatWidth()
				sess.input.Focus()
				sess.focus = FocusEditor
				return m, m.startCodexLoginCmd(pendingModel)
			case rpActionCodexSignOut:
				_ = codex.Delete()
				sess.rightPanel.OpenKeyManager(m.height)
				sess.focus = FocusRightPanel
				sess.input.Blur()
			}
			return m, nil
		}

		// --- Command palette ---
		if m.commandPalette.IsVisible() {
			action, _ := m.commandPalette.Update(msg)
			cmds = append(cmds, m.handleCommandAction(action, sess)...)
			if !m.commandPalette.IsVisible() && sess != nil && sess.focus != FocusRightPanel && m.activeTab != TabKindSessions && m.activeTab != TabKindSettings {
				sess.input.Focus()
				sess.focus = FocusEditor
			}
			return m, tea.Batch(cmds...)
		}

		// --- Global workspace shortcuts ---
		switch msg.String() {
		case "ctrl+n":
			activeModel := m.activeModelForNewSession()
			if m.selectedSession < len(m.sessions)-1 {
				m.selectedSession++
				m.activeTab = TabKindChat
				selSess := m.sessions[m.selectedSession]
				selSess.unreadCount = 0
				selSess.input.SetWidth(m.width - 4)
				if selSess.client == nil && !selSess.reconnecting {
					selSess.reconnecting = true
					cmds = append(cmds, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, false, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, selSess.daemonSessionID))
				}
				cmds = append(cmds, selSess.thinkingAnim.Resume())
				if !m.hasAlertSessions() {
					m.stopTabAlertBlink()
				}
			} else if curSess := m.currentSession(); curSess != nil {
				return m, m.emitStatusMsg("No next session", StatusMsgWarning)
			}
			return m, tea.Batch(cmds...)

		case "ctrl+p":
			activeModel := m.activeModelForNewSession()
			if m.selectedSession > 0 {
				m.selectedSession--
				m.activeTab = TabKindChat
				selSess := m.sessions[m.selectedSession]
				selSess.unreadCount = 0
				selSess.input.SetWidth(m.width - 4)
				if selSess.client == nil && !selSess.reconnecting {
					selSess.reconnecting = true
					cmds = append(cmds, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, false, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, selSess.daemonSessionID))
				}
				cmds = append(cmds, selSess.thinkingAnim.Resume())
				if !m.hasAlertSessions() {
					m.stopTabAlertBlink()
				}
			} else if curSess := m.currentSession(); curSess != nil {
				return m, m.emitStatusMsg("No previous session", StatusMsgWarning)
			}
			return m, tea.Batch(cmds...)

		case "ctrl+t":
			activeModel := m.activeModelForNewSession()
			newSess := newSessionState(m.cfg, nil)
			newSess.modelName = activeModel
			newSess.input.SetWidth(m.width - 4)
			newSess.reconnecting = true
			newIdx := len(m.sessions)
			m.sessions = append(m.sessions, newSess)
			m.selectedSession = newIdx
			m.activeTab = TabKindChat
			m.persistSessions()
			cmds = append(cmds, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, false, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, newSess.daemonSessionID))
			return m, tea.Batch(cmds...)

		}

		// --- Sessions tab key handling ---
		if m.activeTab == TabKindSessions {
			switch msg.String() {
			case "up", "k":
				if m.sessionsSelected > 0 {
					m.sessionsSelected--
				}
				return m, nil
			case "down", "j":
				if n := m.sessionsVisibleCount(); m.sessionsSelected < n-1 {
					m.sessionsSelected++
				}
				return m, nil
			case "pgup":
				if m.sessionsSelected > 5 {
					m.sessionsSelected -= 5
				} else {
					m.sessionsSelected = 0
				}
				return m, nil
			case "pgdown":
				if n := m.sessionsVisibleCount(); m.sessionsSelected+5 < n {
					m.sessionsSelected += 5
				} else if n := m.sessionsVisibleCount(); n > 0 && m.sessionsSelected < n-1 {
					m.sessionsSelected = n - 1
				}
				return m, nil
			case "home", "g":
				m.sessionsSelected = 0
				return m, nil
			case "end", "G":
				if n := m.sessionsVisibleCount(); n > 0 {
					m.sessionsSelected = n - 1
				}
				return m, nil
			case "enter":
				if idx, ok := m.sessionsSelectedIdx(); ok {
					m.selectedSession = idx
					m.activeTab = TabKindChat
					selSess := m.sessions[idx]
					selSess.unreadCount = 0
					selSess.input.SetWidth(m.width - 4)
					selSess.input.Focus()
					selSess.focus = FocusEditor
					m.sessionsInput.Blur()
					if selSess.client == nil {
						// Reconnect the session. This covers both
						// freshly-added sessions and placeholder
						// sessions restored from disk on startup.
						selSess.reconnecting = true
						activeModel := selSess.modelName
						if activeModel == "" {
							activeModel = m.activeModelForNewSession()
						}
						cmds = append(cmds, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, false, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, selSess.daemonSessionID))
					}
					cmds = append(cmds, selSess.thinkingAnim.Resume())
					if !m.hasAlertSessions() {
						m.stopTabAlertBlink()
					}
				} else {
					cmds = append(cmds, m.emitStatusMsg("No session selected", StatusMsgWarning))
				}
				return m, tea.Batch(cmds...)
			case "a":
				// Add a new session
				activeModel := m.activeModelForNewSession()
				newSess := newSessionState(m.cfg, nil)
				newSess.modelName = activeModel
				newSess.input.SetWidth(m.width - 4)
				newSess.reconnecting = true
				newIdx := len(m.sessions)
				m.sessions = append(m.sessions, newSess)
				m.selectedSession = newIdx
				m.activeTab = TabKindChat
				m.persistSessions()
				cmds = append(cmds, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, false, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, newSess.daemonSessionID))
				return m, tea.Batch(cmds...)
			case "x":
				if idx, ok := m.sessionsSelectedIdx(); ok {
					m.sessionCloseIdx = idx
					m.sessionCloseSelected = 1 // default No
					m.state = StateSessionCloseConfirm
				}
				return m, nil
			case "f1":
				// Already on the Sessions tab — clear the filter and reset
				// the cursor to the active session so the user lands on a
				// known-good row.
				ti := m.sessionsInput
				ti.SetValue("")
				m.sessionsInput = ti
				m.sessionsInput.Blur()
				m.sessionsInput.Focus()
				m.syncSessionsSelected()
				return m, m.sessionsInput.Focus()
			case "f2":
				m.activeTab = TabKindChat
				if sess := m.currentSession(); sess != nil {
					sess.unreadCount = 0
					sess.input.Focus()
					sess.focus = FocusEditor
					m.sessionsInput.Blur()
					cmds = append(cmds, sess.thinkingAnim.Resume())
				}
				return m, tea.Batch(cmds...)
			case "f3":
				m.openSettingsTab()
				m.sessionsInput.Blur()
				return m, tea.Batch(cmds...)
			case "esc":
				ti := m.sessionsInput
				ti.SetValue("")
				m.sessionsInput = ti
				m.sessionsInput.Blur()
				m.sessionsInput.Focus()
				m.syncSessionsSelected()
				return m, nil
			default:
				var cmd tea.Cmd
				m.sessionsInput, cmd = m.sessionsInput.Update(msg)
				if n := m.sessionsVisibleCount(); n > 0 && m.sessionsSelected >= n {
					m.sessionsSelected = n - 1
				}
				return m, cmd
			}
		}

		// --- Settings tab key handling ---
		if m.activeTab == TabKindSettings {
			if m.settingsInKeyInput {
				switch msg.String() {
				case "esc":
					m.settingsInKeyInput = false
					m.settingsInputMode = settingsInputNone
					m.settingsModelPending = ""
					m.refreshSettingsKeys()
				case "enter":
					val := strings.TrimSpace(m.settingsKeyInput.Value())
					mode := m.settingsInputMode
					providerName := m.settingsKeyInputProvider
					m.settingsInKeyInput = false
					m.settingsInputMode = settingsInputNone
					switch mode {
					case settingsInputAPIKey:
						if val != "" && m.cortexCfg != nil {
							m.cortexCfg.SetProviderAPIKey(providerName, val)
							_ = cortexconfig.Save(m.cortexCfg)
						}
						m.refreshSettingsKeys()
						if m.settingsModelPending != "" && val != "" {
							pending := m.settingsModelPending
							m.settingsModelPending = ""
							m.setActiveModelSpec(pending)
							if m.cortexCfg != nil {
								m.cortexCfg.DefaultModel = pending
								_ = cortexconfig.Save(m.cortexCfg)
							}
							if pendSess := m.currentSession(); pendSess != nil && pendSess.client != nil {
								_ = pendSess.client.SendSetModel(pending)
							}
						} else {
							m.settingsModelPending = ""
						}
						if val != "" {
							cmds = append(cmds, m.emitStatusMsg("API key saved for "+providerName, StatusMsgInfo), m.fetchModelsForProvider(providerName))
						}
					case settingsInputBaseURL:
						if val != "" && m.cortexCfg != nil {
							m.cortexCfg.SetProviderBaseURL(providerName, val)
							_ = cortexconfig.Save(m.cortexCfg)
							cmds = append(cmds, m.emitStatusMsg("Base URL saved for "+providerName, StatusMsgInfo), m.fetchModelsForProvider(providerName))
						}
						m.refreshSettingsKeys()
					case settingsInputCustomProviderName:
						providerName = cortexconfig.NormalizeProviderName(val)
						if providerName == "" {
							cmds = append(cmds, m.emitStatusMsg("Provider name cannot be empty", StatusMsgError))
						} else {
							m.settingsCustomProvider = providerName
							m.openSettingsTextInput(settingsInputCustomProviderBaseURL, providerName, "New provider base URL", "https://example.com/v1", "", false)
						}
					case settingsInputCustomProviderBaseURL:
						if val == "" {
							cmds = append(cmds, m.emitStatusMsg("Base URL cannot be empty", StatusMsgError))
						} else {
							m.settingsCustomBaseURL = val
							m.openSettingsTextInput(settingsInputCustomProviderAPIKey, providerName, "New provider API key", "Paste API key (optional)...", "", true)
						}
					case settingsInputCustomProviderAPIKey:
						if m.settingsCustomProvider != "" && m.settingsCustomBaseURL != "" && m.cortexCfg != nil {
							providerName = m.cortexCfg.AddCustomProvider(m.settingsCustomProvider, m.settingsCustomBaseURL, val)
							_ = cortexconfig.Save(m.cortexCfg)
							m.refreshSettingsKeys()
							providers := m.settingsProviders()
							for i, p := range providers {
								if p.Name == providerName {
									m.settingsProviderSel = i
									m.settingsKeySel = i
									break
								}
							}
							m.settingsActiveSection = 1
							cmds = append(cmds, m.emitStatusMsg("Custom provider added: "+providerName, StatusMsgInfo), m.fetchModelsForProvider(providerName))
						}
					default:
						m.refreshSettingsKeys()
					}
				default:
					var cmd tea.Cmd
					m.settingsKeyInput, cmd = m.settingsKeyInput.Update(msg)
					cmds = append(cmds, cmd)
				}
			} else if m.settingsActiveSection == 0 {
				// Model section — two columns: providers (column 0) and
				// per-provider model list (column 1).
				providers := m.settingsProviders()
				clampProviderSel := func() {
					if m.settingsProviderSel < 0 {
						m.settingsProviderSel = 0
					}
					if len(providers) == 0 {
						m.settingsProviderSel = 0
						return
					}
					if m.settingsProviderSel >= len(providers) {
						m.settingsProviderSel = len(providers) - 1
					}
				}
				clampModelSel := func() {
					models := m.selectedSettingsModels()
					if m.settingsModelSel < 0 {
						m.settingsModelSel = 0
					}
					if len(models) == 0 {
						m.settingsModelSel = 0
						return
					}
					if m.settingsModelSel >= len(models) {
						m.settingsModelSel = len(models) - 1
					}
				}
				clampProviderSel()
				switch msg.String() {
				case "up", "k":
					if m.settingsModelColumn == 0 {
						if m.settingsProviderSel > 0 {
							m.settingsProviderSel--
							m.settingsModelSel = 0
						}
					} else {
						if m.settingsModelSel > 0 {
							m.settingsModelSel--
						}
					}
				case "down", "j":
					if m.settingsModelColumn == 0 {
						if m.settingsProviderSel < len(providers)-1 {
							m.settingsProviderSel++
							m.settingsModelSel = 0
						}
					} else {
						models := m.selectedSettingsModels()
						if m.settingsModelSel < len(models)-1 {
							m.settingsModelSel++
						}
					}
				case "left", "h":
					m.settingsModelColumn = 0
				case "right", "l":
					m.settingsModelColumn = 1
					clampModelSel()
				case "r":
					providerName := m.selectedSettingsProviderName()
					if providerName != "" {
						cmds = append(cmds, m.emitStatusMsg("Refreshing models for "+providerName, StatusMsgInfo), m.fetchModelsForProvider(providerName))
					}
				case "enter":
					clampProviderSel()
					if m.settingsModelColumn == 0 {
						// Enter on the provider column jumps to the model column.
						m.settingsModelColumn = 1
						m.settingsModelSel = 0
						clampModelSel()
					} else {
						models := m.selectedSettingsModels()
						if len(models) > 0 && m.settingsModelSel < len(models) {
							mod := models[m.settingsModelSel]
							if m.settingsProviderHasKey(mod.Provider) {
								cmds = append(cmds, m.selectSettingsModel(mod))
							} else {
								m.settingsModelPending = mod.Spec
								m.openAPIKeyInput(mod.Provider)
							}
						}
					}
				case "tab":
					m.settingsActiveSection = 1
				case "f1":
					m.activeTab = TabKindSessions
					m.syncSessionsSelected()
					cmds = append(cmds, m.sessionsInput.Focus())
				case "f2":
					m.activeTab = TabKindChat
					if s := m.currentSession(); s != nil {
						s.unreadCount = 0
						cmds = append(cmds, s.thinkingAnim.Resume())
					}
				}
			} else if m.settingsActiveSection == 1 {
				// Provider/API section
				m.refreshSettingsKeys()
				if m.settingsWizard.active {
					// Wizard is open. It owns the entire Providers
					// section. Only arrows, Enter, and Esc are
					// meaningful inside it.
					w := &m.settingsWizard
					key := msg.String()
					switch key {
					case "esc":
						cmds = append(cmds, m.closeSettingsWizard())
					case "enter":
						cmds = append(cmds, m.wizardCommitCurrent())
					case "up":
						cmds = append(cmds, m.wizardMoveField(-1))
					case "down":
						cmds = append(cmds, m.wizardMoveField(+1))
					default:
						// Any other printable key feeds the text input.
						// We skip non-character events (e.g. release
						// events) by routing only the keys the
						// bubbles textinput understands.
						if key != "" && key != "ctrl+c" && key != "ctrl+d" {
							var cmd tea.Cmd
							w.input, cmd = w.input.Update(msg)
							if cmd != nil {
								cmds = append(cmds, cmd)
							}
						}
					}
					return m, tea.Batch(cmds...)
				}
				switch msg.String() {
				case "up", "k":
					if m.settingsKeySel > 0 {
						m.settingsKeySel--
					}
				case "down", "j":
					if m.settingsKeySel < len(m.settingsKeys)-1 {
						m.settingsKeySel++
					}
				case "enter":
					if m.settingsKeySel < len(m.settingsKeys) {
						m.openSettingsWizard(m.settingsKeys[m.settingsKeySel].Provider)
					}
				case "a":
					m.openSettingsTextInput(settingsInputCustomProviderName, "", "New provider name", "e.g. groq, together, local-ai", "", false)
				case "r":
					if m.settingsKeySel < len(m.settingsKeys) {
						providerName := m.settingsKeys[m.settingsKeySel].Provider
						cmds = append(cmds, m.emitStatusMsg("Refreshing models for "+providerName, StatusMsgInfo), m.fetchModelsForProvider(providerName))
					}
				case "tab":
					m.settingsActiveSection = 2
				case "f1":
					m.activeTab = TabKindSessions
					m.syncSessionsSelected()
					cmds = append(cmds, m.sessionsInput.Focus())
				case "f2":
					m.activeTab = TabKindChat
					if s := m.currentSession(); s != nil {
						s.unreadCount = 0
						cmds = append(cmds, s.thinkingAnim.Resume())
					}
				}
			} else {
				// Other Settings section
				switch msg.String() {
				case "up", "k":
					if m.settingsOtherSel > 0 {
						m.settingsOtherSel--
					}
				case "down", "j":
					if m.settingsOtherSel < settingsOtherOptionCount-1 {
						m.settingsOtherSel++
					}
				case "enter":
					switch m.settingsOtherSel {
					case 0: // Theme — cycle auto → dark → light → auto
						m.setConfiguredTheme(nextTheme(m.configuredTheme()))
						cmds = append(cmds, m.emitStatusMsg("Theme: "+m.configuredTheme(), StatusMsgInfo))
					case 1: // Show extended thinking — toggle
						if sess := m.currentSession(); sess != nil {
							sess.showThinking = !sess.showThinking
							if sess.showThinking && sess.thinkingBuf != "" {
								sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
							} else {
								sess.thinkingRendered = ""
							}
							_ = config.SetShowThinking(sess.showThinking)
						}
					case 2: // Reasoning effort — cycle auto → low → medium → high
						m.setActiveReasoningEffort(nextReasoningEffort(m.currentReasoningEffort()))
						cmds = append(cmds, m.emitStatusMsg("Reasoning effort: "+m.currentReasoningEffort(), StatusMsgInfo))
					case 3: // Streaming — toggle
						m.setConfiguredStreaming(!m.configuredStreaming())
						state := "on"
						if !m.configuredStreaming() {
							state = "off"
						}
						cmds = append(cmds, m.emitStatusMsg("Streaming responses: "+state, StatusMsgInfo))
					case 4: // Show token usage — toggle
						m.setConfiguredShowUsage(!m.configuredShowUsage())
						state := "on"
						if !m.configuredShowUsage() {
							state = "off"
						}
						cmds = append(cmds, m.emitStatusMsg("Show token usage: "+state, StatusMsgInfo))
					}
				case "tab":
					m.settingsActiveSection = 0
				case "f1":
					m.activeTab = TabKindSessions
					m.syncSessionsSelected()
					cmds = append(cmds, m.sessionsInput.Focus())
				case "f2":
					m.activeTab = TabKindChat
					if s := m.currentSession(); s != nil {
						s.unreadCount = 0
						cmds = append(cmds, s.thinkingAnim.Resume())
					}
				}
			}
			return m, tea.Batch(cmds...)
		}

		// --- Chat tab key handling (session-specific) ---
		if sess == nil {
			return m, nil
		}

		// Attachment panel intercepts keys when focused
		if sess.attachmentPanel.IsFocused() {
			switch msg.String() {
			case "up", "k":
				sess.attachmentPanel.MoveUp()
			case "down", "j":
				sess.attachmentPanel.MoveDown()
			case "delete", "backspace":
				sess.attachmentPanel.Remove(sess.attachmentPanel.selected)
			case "enter":
				// prevent submit
			case "tab":
				sess.attachmentPanel.Unfocus()
				sess.focus = FocusChat
				sess.input.Blur()
			case "esc":
				sess.attachmentPanel.Unfocus()
				sess.focus = FocusEditor
				sess.input.Focus()
			default:
				sess.attachmentPanel.Unfocus()
				sess.input.Focus()
				goto processKey
			}
			return m, nil
		}
	processKey:

		// Slash menu
		if sess.slashMenu.IsVisible() {
			switch msg.String() {
			case "up":
				sess.slashMenu.MoveUp()
				return m, nil
			case "down":
				sess.slashMenu.MoveDown()
				return m, nil
			case "esc":
				sess.slashMenu.Close()
				return m, nil
			case "enter", "tab":
				action := sess.slashMenu.SelectedAction()
				sess.slashMenu.Close()
				sess.input.SetValue("")
				sess.input.SetHeight(1)
				if action != "" {
					cmds = append(cmds, m.handleCommandAction(action, sess)...)
				}
				if sess.focus != FocusRightPanel && m.activeTab != TabKindSessions && m.activeTab != TabKindSettings {
					sess.input.Focus()
					sess.focus = FocusEditor
				}
				return m, tea.Batch(cmds...)
			}
		}

		// File completer
		if sess.fileCompleter.IsVisible() {
			switch msg.String() {
			case "up":
				sess.fileCompleter.MoveUp()
				return m, nil
			case "down":
				sess.fileCompleter.MoveDown()
				return m, nil
			case "esc":
				sess.fileCompleter.Close()
				return m, nil
			case "enter", "tab":
				entry := sess.fileCompleter.SelectedEntry()
				if entry == nil {
					sess.fileCompleter.Close()
					return m, nil
				}
				if entry.IsDir() {
					sess.fileCompleter.Descend(entry)
					newPath := "@" + sess.fileCompleter.currentDir + "/"
					sess.input.SetValue(replaceAtToken(sess.input.Value(), newPath))
					sess.input.MoveToEnd()
				} else {
					path := sess.fileCompleter.SelectedPath()
					sess.input.SetValue(replaceAtToken(sess.input.Value(), path))
					sess.input.MoveToEnd()
					sess.fileCompleter.Close()
				}
				newHeight := m.visualLineCount()
				if newHeight != sess.input.Height() {
					sess.input.SetHeight(newHeight)
				}
				return m, nil
			}
		}

		// Tab key: when the user is mid-turn and the input editor
		// has text, Tab queues the message to be sent on the next
		// turn (no cancel). This is the dual of Enter, which
		// queues the message and interrupts the current turn
		// after the in-flight edit finishes. When there is no
		// pending text (or we are not streaming), Tab keeps its
		// existing focus-cycling behavior.
		if msg.String() == "tab" {
			if sess.focus == FocusEditor && (sess.agentState == StateStreaming ||
				sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting) &&
				strings.TrimSpace(sess.input.Value()) != "" {
				return m.submitFromInput(sess, true)
			}
			if sess.agentState == StateWaitingForInput || sess.agentState == StatePlanReview ||
				sess.agentState == StateUserQuestion || sess.agentState == StateStreaming ||
				sess.agentState == StateToolExecuting || sess.agentState == StateConfirmPending {
				switch sess.focus {
				case FocusEditor:
					if sess.attachmentPanel.IsVisible() {
						sess.attachmentPanel.Focus()
						sess.input.Blur()
					} else {
						sess.focus = FocusChat
						sess.input.Blur()
					}
				case FocusChat:
					if sess.rightPanel.IsVisible() {
						sess.focus = FocusRightPanel
						sess.input.Blur()
					} else {
						sess.focus = FocusEditor
						sess.input.Focus()
					}
				case FocusRightPanel:
					sess.focus = FocusEditor
					sess.input.Focus()
				}
			}
			return m, nil
		}

		// Question / confirm panel
		if (sess.agentState == StateUserQuestion || sess.agentState == StateConfirmPending) &&
			sess.questionPanel.IsVisible() && sess.focus == FocusEditor {
			result, answer, batchAnswers := sess.questionPanel.HandleKey(msg)
			switch result {
			case QPSubmitted:
				if sess.agentState == StateConfirmPending {
					approved := answer == "Yes, allow" || answer == "Allow once" || answer == "Allow and remember"
					persistDirs := answer == "Allow and remember"
					question := sess.questionPanel.CurrentTab().Question
					pairs := []QAPair{{Category: "Permission", Question: question, Answer: answer}}
					sess.chatMessages = append(sess.chatMessages, renderQuestionAnswer(pairs, m.styles))
					if sess.client != nil {
						sess.client.SendConfirm(approved, persistDirs)
					}
					sess.questionPanel.Close()
					sess.agentState = StateToolExecuting
					return m, sess.thinkingAnim.Start()
				}
				if batchAnswers != nil {
					pairs := sess.questionPanel.GetAnsweredPairs()
					sess.chatMessages = append(sess.chatMessages, renderQuestionAnswer(pairs, m.styles))
					if sess.client != nil {
						sess.client.SendUserAnswerBatch(batchAnswers)
					}
				} else {
					answerText := sess.questionPanel.CurrentAnswerText()
					tab := sess.questionPanel.CurrentTab()
					displayAnswer := answer
					if answerText != "" {
						displayAnswer = answer + ": " + answerText
					}
					pairs := []QAPair{{Category: tab.Category, Question: tab.Question, Answer: displayAnswer}}
					sess.chatMessages = append(sess.chatMessages, renderQuestionAnswer(pairs, m.styles))
					if sess.client != nil {
						sess.client.SendUserAnswer(answer, answerText)
					}
				}
				sess.questionPanel.Close()
				sess.agentState = StateStreaming
				return m, sess.thinkingAnim.Start()
			case QPCancelled:
				if sess.agentState == StateConfirmPending {
					pairs := []QAPair{{Category: "Permission", Question: sess.questionPanel.CurrentTab().Question, Answer: "Deny"}}
					sess.chatMessages = append(sess.chatMessages, renderQuestionAnswer(pairs, m.styles))
					if sess.client != nil {
						sess.client.SendConfirm(false, false)
					}
					sess.questionPanel.Close()
					sess.agentState = StateToolExecuting
					return m, sess.thinkingAnim.Start()
				}
				if sess.client != nil {
					sess.client.SendUserAnswer("", "")
				}
				sess.questionPanel.Close()
				sess.agentState = StateStreaming
				return m, sess.thinkingAnim.Start()
			}
			return m, nil
		}

		// Shift+Enter / Alt+Enter: newline
		if msg.String() == "shift+enter" || msg.String() == "alt+enter" || msg.String() == "ctrl+j" {
			if sess.agentState == StateWaitingForInput || sess.agentState == StatePlanReview ||
				sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
				sess.input.InsertString("\n")
				newHeight := m.visualLineCount()
				if newHeight != sess.input.Height() {
					sess.input.SetHeight(newHeight)
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+shift+u":
			if sess.agentState == StateWaitingForInput || sess.agentState == StatePlanReview ||
				sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
				sess.input.SetValue("")
				sess.input.SetHeight(1)
			}
			return m, nil

		case "ctrl+r":
			if sess.agentState == StateWaitingForInput && len(sess.history.entries) > 0 {
				sess.historyPanel.Open(len(sess.history.entries), m.height)
			}
			return m, nil

		case "f1":
			m.activeTab = TabKindSessions
			m.syncSessionsSelected()
			cmds = append(cmds, m.sessionsInput.Focus())
			return m, tea.Batch(cmds...)

		case "f2":
			m.activeTab = TabKindChat
			if sess := m.currentSession(); sess != nil {
				sess.unreadCount = 0
			}
			return m, tea.Batch(cmds...)

		case "f3":
			m.openSettingsTab()
			return m, tea.Batch(cmds...)

		case "shift+tab":
			if sess.agentState == StateWaitingForInput && len(sess.workflows) > 0 {
				sess.activeWorkflow = m.nextWorkflow(sess)
				sess.input.Placeholder = m.placeholderForMode(sess)
				m.updateInputPromptColor(sess)
				return m, m.emitStatusMsg("Context is not shared between Chat and workflows", StatusMsgInfo)
			}
			return m, nil

		case "enter":
			return m.handleEnter(sess)

		case "y", "Y":
			if sess.agentState == StatePlanReview && sess.input.Value() == "" {
				if sess.reconnecting {
					sess.pendingPlanAction = &pendingPlanAction{action: "approve"}
					return m, nil
				}
				if sess.client != nil {
					sess.client.SendPlanAction("approve", "")
				}
				sess.agentState = StateStreaming
				return m, sess.thinkingAnim.Start()
			}

		case "esc":
			if sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
				sess.thinkingAnim.Stop()
				sess.pendingInput = nil
				if sess.client != nil {
					sess.client.SendCancel()
				}
				m.flushSessionBuf(sess)
				sess.chatMessages = append(sess.chatMessages, renderSystemMessage("Cancelled.", m.styles))
				return m, nil
			}
			if sess.agentState == StatePlanReview && sess.input.Value() == "" {
				if sess.reconnecting {
					sess.pendingPlanAction = &pendingPlanAction{action: "reject"}
					return m, nil
				}
				if sess.client != nil {
					sess.client.SendPlanAction("reject", "")
				}
				sess.agentState = StateWaitingForInput
				sess.input.Focus()
				return m, nil
			}

		case "n", "N":
			if sess.agentState == StatePlanReview && sess.input.Value() == "" {
				if sess.reconnecting {
					sess.pendingPlanAction = &pendingPlanAction{action: "reject"}
					return m, nil
				}
				if sess.client != nil {
					sess.client.SendPlanAction("reject", "")
				}
				sess.agentState = StateWaitingForInput
				sess.input.Focus()
				return m, nil
			}
		}

		// Chat viewport focus: scroll keys
		if sess.focus == FocusChat {
			switch msg.String() {
			case "up", "k":
				sess.chatScrollOffset += 3
			case "down", "j":
				sess.chatScrollOffset -= 3
			case "pgup", "b":
				sess.chatScrollOffset += 20
			case "pgdown", "f":
				sess.chatScrollOffset -= 20
			case "home", "g":
				sess.chatScrollOffset = m.sessionMaxScrollOffset(sess)
			case "end", "G":
				sess.chatScrollOffset = 0
			case "F":
				if sep, ok := m.sessionActiveForkSep(sess); ok {
					return m.doFork(sep)
				}
			case "T":
				if sep, ok := m.sessionActiveForkSep(sess); ok {
					sess.trimPrevState = sess.agentState
					sess.trimSelected = 0
					sess.trimSep = sep
					sess.agentState = StateTrimConfirm
					return m, nil
				}
			}
			m.clampScrollOffset(sess)
			return m, nil
		}

		if sess.agentState == StateWaitingForInput || sess.agentState == StatePlanReview ||
			sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
			if msg.String() == "up" && sess.agentState == StateWaitingForInput &&
				sess.input.Line() == 0 && sess.input.Column() == 0 && len(sess.history.entries) > 0 {
				sess.historyPanel.Open(len(sess.history.entries), m.height)
				return m, nil
			}
			var cmd tea.Cmd
			sess.input, cmd = sess.input.Update(msg)

			query, found := extractAtQuery(sess.input.Value())
			if found {
				dir, prefix := resolveAtDir(query, m.cwd)
				if sess.fileCompleter.IsVisible() && dir == sess.fileCompleter.currentDir {
					sess.fileCompleter.Refresh(prefix)
				} else {
					sess.fileCompleter.Open(dir, prefix)
				}
			} else {
				sess.fileCompleter.Close()
			}

			slashQuery, slashFound := extractSlashQuery(sess.input.Value())
			if slashFound {
				if sess.slashMenu.IsVisible() {
					sess.slashMenu.Refresh(slashQuery)
				} else {
					sess.slashMenu.Open(slashCommands, slashQuery)
				}
			} else {
				sess.slashMenu.Close()
			}

			newHeight := m.visualLineCount()
			if newHeight != sess.input.Height() {
				sess.input.SetHeight(newHeight)
			}
			return m, cmd
		}

		return m, nil

	// --- Session daemon events ---
	case sessionEventMsg:
		idx, sess := m.findSessionByDaemonID(msg.daemonSessionID)
		if sess != nil {
			evCmds := m.applyEventToSession(idx, msg.event)
			cmds = append(cmds, evCmds...)
			cmds = append(cmds, m.maybeStartTabAlertBlink())
		}
		return m, tea.Batch(cmds...)

	case sessionDisconnectedMsg:
		_, sess := m.findSessionByDaemonID(msg.daemonSessionID)
		if sess != nil {
			sess.reconnecting = true
			sess.pendingInput = nil
			sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("daemon connection lost")))
			if sess.agentState != StatePlanReview {
				sess.agentState = StateWaitingForInput
			}
			activeModel := sess.modelName
			if activeModel == "" {
				activeModel = m.activeModelForNewSession()
			}
			cmds = append(cmds, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, m.forceInit, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, msg.daemonSessionID))
		}
		return m, tea.Batch(cmds...)

	case reconnectSuccessMsg:
		_, sess := m.findSessionByDaemonID(msg.daemonSessionID)
		if sess == nil {
			// Session was closed while the reconnect goroutine was in flight.
			// Close the new client to avoid leaking a daemon-side session.
			msg.client.Close()
			return m, nil
		}
		// Close the previous client before replacing it so the old event-loop
		// goroutine unblocks and exits cleanly.
		if sess.client != nil {
			sess.client.Close()
		}
		sess.client = msg.client
		sess.daemonSessionID = msg.client.SessionID()
		sess.reconnecting = false
		if len(sess.chatMessages) > 0 {
			sess.chatMessages = append(sess.chatMessages, renderSystemSuccessMessage("Reconnected to daemon."))
		}
		// Seed the freshly-reconnected daemon with the prior chat
		// scrollback so the user's next message has full
		// conversation context, not just the new question in
		// isolation. We only restore if there is actual content
		// (not just the "reconnected" system message we just
		// appended). We skip the very last message in case it
		// is the reconnection banner we just added.
		if len(sess.chatMessages) > 0 {
			// Drop the reconnection banner before sending;
			// the UI re-adds it on every reconnect so we
			// must not let it leak into the provider
			// history.
			history := sess.chatMessages
			if history[len(history)-1].Type == MsgSystem {
				history = history[:len(history)-1]
			}
			_ = sess.client.SendRestoreHistory(chatMessagesToProviderHistory(history))
		}
		if sess.pendingPlanAction != nil {
			pending := sess.pendingPlanAction
			sess.pendingPlanAction = nil
			sess.client.SendPlanAction(pending.action, pending.text)
			sess.agentState = StateStreaming
			return m, tea.Batch(startSessionEventLoop(msg.client), sess.thinkingAnim.Start())
		}
		return m, startSessionEventLoop(msg.client)

	case reconnectFailedMsg:
		// Don't retry if the session has never successfully connected — there
		// is no stable daemonSessionID to match against, and a brand-new
		// session that failed its first attempt should not loop indefinitely.
		if msg.daemonSessionID == "" {
			return m, nil
		}
		_, sess := m.findSessionByDaemonID(msg.daemonSessionID)
		if sess != nil && sess.reconnecting {
			activeModel := sess.modelName
			if activeModel == "" {
				activeModel = m.activeModelForNewSession()
			}
			return m, attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, m.forceInit, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, msg.daemonSessionID)
		}
		return m, nil

	case tea.PasteMsg:
		if m.activeTab == TabKindSettings && m.settingsInKeyInput {
			m.settingsKeyInput, _ = m.settingsKeyInput.Update(msg)
			return m, nil
		}
		sess := m.currentSession()
		if sess == nil {
			return m, nil
		}
		if sess.rightPanel.IsVisible() && sess.focus == FocusRightPanel && sess.rightPanel.mode == rpModeKeyInput {
			sess.rightPanel.keyInput, _ = sess.rightPanel.keyInput.Update(msg)
			return m, nil
		}
		if sess.agentState == StateWaitingForInput || sess.agentState == StatePlanReview ||
			sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
			var cmd tea.Cmd
			sess.input, cmd = sess.input.Update(msg)
			val := sess.input.Value()
			_, atts, _ := extractImageAttachments(val)
			if len(atts) > 0 {
				for i := range atts {
					sess.attachmentPanel.Add(atts[i])
				}
				stripped := imagePathPattern.ReplaceAllString(val, "")
				stripped = strings.TrimSpace(stripped)
				sess.input.SetValue(stripped)
			}
			newHeight := m.visualLineCount()
			if newHeight != sess.input.Height() {
				sess.input.SetHeight(newHeight)
			}
			sess.input.MoveToBegin()
			sess.input.MoveToEnd()
			return m, cmd
		}

	case tea.KeyboardEnhancementsMsg:
		m.kittySupported = msg.SupportsKeyDisambiguation()

	case tea.BackgroundColorMsg:
		m.hasDarkBG = msg.IsDark()
		m.styles = NewStyles(m.hasDarkBG)
		m.mdRenderer = NewMarkdownRenderer(m.mdRenderer.width, m.hasDarkBG, m.styles.CodeBoxBorderStyle)
		return m, nil

	case resumeFromSleepMsg:
		return m, tea.Batch(tea.ClearScreen, tea.RequestWindowSize, waitForResume)

	case clearStatusMsgMsg:
		if msg.gen == m.statusMsg.gen {
			m.statusMsg = StatusMessage{}
		}
		return m, nil

	case startCursorBlinkMsg:
		sess := m.currentSession()
		if sess != nil {
			blinkCmd := sess.input.Focus()
			return m, blinkCmd
		}
		return m, nil

	case sessionTitleGeneratedMsg:
		// AI (or fallback) produced a title for one of our sessions.
		// Apply it to the matching session, persist, and refresh the
		// Sessions tab so the new label shows up. If the session was
		// already named (e.g. user manually renamed it during the AI
		// call) we keep the existing label.
		if msg.title == "" {
			return m, nil
		}
		for _, s := range m.sessions {
			if s.persistID != "" && s.persistID == msg.sessionID {
				if s.label == "" {
					s.label = msg.title
					m.persistSessions()
				}
				break
			}
		}
		return m, nil

	case modelsFetchedMsg:
		providerName := cortexconfig.NormalizeProviderName(msg.provider)
		if msg.err != nil {
			return m, m.emitStatusMsg("Failed to refresh models for "+providerName+": "+msg.err.Error(), StatusMsgError)
		}
		if len(msg.models) == 0 {
			return m, m.emitStatusMsg("No models returned for "+providerName, StatusMsgWarning)
		}
		if m.cortexCfg != nil {
			if mc, ok := m.cortexCfg.Models[providerName]; ok && (mc.Model == "" || mc.Model == "model") {
				mc.Model = msg.models[0]
				m.cortexCfg.Models[providerName] = mc
			}
			for _, modelID := range msg.models {
				m.cortexCfg.EnsureProviderModel(providerName, modelID)
			}
			_ = cortexconfig.Save(m.cortexCfg)
		}
		m.refreshSettingsKeys()
		if providerName == m.selectedSettingsProviderName() && m.settingsModelSel >= len(m.selectedSettingsModels()) {
			m.settingsModelSel = 0
		}
		return m, m.emitStatusMsg(fmt.Sprintf("Loaded %d model(s) for %s", len(msg.models), providerName), StatusMsgInfo)

	case codexLoginSuccessMsg:
		return m, m.handleCodexLoginSuccess(msg.pendingModel, msg.email, msg.planType)

	case codexLoginFailedMsg:
		return m, m.handleCodexLoginFailed(msg.err, msg.authorizeURL)

	case animStepMsg:
		// Route to whichever session owns this generation tick.
		for _, sess := range m.sessions {
			if cmd := sess.thinkingAnim.Advance(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case tabBlinkMsg:
		if msg.gen != m.tabAlertBlinkGen {
			return m, nil
		}
		m.tabAlertBlinkOn = !m.tabAlertBlinkOn
		if m.hasAlertSessions() {
			return m, m.tabBlinkTick()
		}
		m.tabAlertActive = false
		m.tabAlertBlinkOn = false
		m.tabAlertBlinkGen++
		return m, nil
	}

	// Forward unhandled messages to the active input for cursor blink
	sess := m.currentSession()
	if m.activeTab == TabKindSessions {
		var cmd tea.Cmd
		m.sessionsInput, cmd = m.sessionsInput.Update(msg)
		if cmd != nil {
			return m, cmd
		}
	} else if sess != nil {
		var cmd tea.Cmd
		sess.input, cmd = sess.input.Update(msg)
		if cmd != nil {
			return m, cmd
		}
	}
	return m, nil
}

// handleDialogKey handles keys for the global quit/session-close dialogs.
func (m Model) handleDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "right", "tab":
		if m.state == StateQuitConfirm {
			if m.quitSelected == 0 {
				m.quitSelected = 1
			} else {
				m.quitSelected = 0
			}
		} else {
			if m.sessionCloseSelected == 0 {
				m.sessionCloseSelected = 1
			} else {
				m.sessionCloseSelected = 0
			}
		}
	case "enter":
		if m.state == StateQuitConfirm {
			if m.quitSelected == 0 {
				sess := m.currentSession()
				if sess != nil && sess.client != nil {
					sess.client.SendCancel()
					sess.client.SendClose()
				}
				// Flush the latest chat scrollback to disk so the
				// user does not lose unsaved messages on quit.
				m.persistSessions()
				return m, tea.Quit
			}
			m.state = StateWaitingForInput
		} else {
			if m.sessionCloseSelected == 0 {
				return m.doCloseSession(m.sessionCloseIdx)
			}
			m.state = StateWaitingForInput
		}
	case "y", "Y":
		if m.state == StateQuitConfirm {
			sess := m.currentSession()
			if sess != nil && sess.client != nil {
				sess.client.SendCancel()
				sess.client.SendClose()
			}
			m.persistSessions()
			return m, tea.Quit
		}
		if m.state == StateSessionCloseConfirm {
			return m.doCloseSession(m.sessionCloseIdx)
		}
	case "n", "N", "esc":
		m.state = StateWaitingForInput
	}
	return m, nil
}

// handleTrimKey handles keys for the per-session trim confirm dialog.
func (m Model) handleTrimKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	sess := m.currentSession()
	if sess == nil {
		return m, nil
	}
	switch msg.String() {
	case "left", "right", "tab":
		if sess.trimSelected == 0 {
			sess.trimSelected = 1
		} else {
			sess.trimSelected = 0
		}
	case "enter":
		if sess.trimSelected == 0 {
			return m.doTrim(sess.trimSep)
		}
		sess.agentState = sess.trimPrevState
	case "y", "Y":
		return m.doTrim(sess.trimSep)
	case "n", "N", "esc":
		sess.agentState = sess.trimPrevState
	}
	return m, nil
}

// submitFromInput takes the current text in sess.input and either
// sends it immediately (queueOnly=false) or queues it for the next
// turn (queueOnly=true). When queueOnly=false, the in-flight turn
// is cancelled (delayed so an in-progress edit can complete first);
// when queueOnly=true, no cancel is requested and the message is
// sent after the current turn finishes naturally.
//
// Both modes render the user message in the chat scrollback so the
// user can see what they typed is "on the way", and they both clear
// the input box.
func (m Model) submitFromInput(sess *SessionState, queueOnly bool) (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(sess.input.Value())
	if text == "" && sess.attachmentPanel.Count() == 0 {
		return m, nil
	}
	if text != "" {
		sess.history.Save(text)
	}
	sess.input.Reset()
	sess.input.SetHeight(1)

	panelAtts := sess.attachmentPanel.Clear()
	displayText, textAtts, imgErrs := extractImageAttachments(text)
	for _, e := range imgErrs {
		sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("%s", e)))
	}
	attachments := append(panelAtts, textAtts...)

	sess.chatMessages = append(sess.chatMessages, renderUserMessage(displayText, m.mdRenderer.width))
	sess.chatScrollOffset = 0

	// For workflow mode we just forward the message and let the
	// current run continue; there is no concept of a "queue".
	if sess.activeWorkflow != "" && !strings.HasPrefix(displayText, "/") && sess.agentState != StatePlanExecuting {
		if sess.client != nil {
			sess.client.SendWorkflowMessage(displayText)
		}
		return m, nil
	}

	// Persist the latest chat scrollback so the queued message
	// is on disk in case the user kills the process before the
	// turn finishes.
	defer m.persistSessions()

	if queueOnly {
		// Tab: queue for after the current turn. No cancel
		// requested — the in-flight response continues to
		// completion, then agent_done flushes pendingInput.
		sess.pendingInput = &pendingMsg{text: displayText, attachments: attachments, Queued: true}
		sess.input.Placeholder = m.placeholderForMode(sess)
		return m, m.emitStatusMsg("Queued for after the current turn. Press Enter to interrupt and send now.", StatusMsgInfo)
	}

	// Enter: queue AND request a delayed cancel so the current
	// edit can complete cleanly before the turn is interrupted.
	sess.pendingInput = &pendingMsg{text: displayText, attachments: attachments, Queued: false}
	sess.input.Placeholder = m.placeholderForMode(sess)
	if sess.client != nil {
		sess.client.SendCancelAfterEdit()
	}
	return m, m.emitStatusMsg("Interrupting after the current edit. Your message will be sent next.", StatusMsgInfo)
}

// handleEnter handles the Enter key in the Chat tab.
func (m Model) handleEnter(sess *SessionState) (tea.Model, tea.Cmd) {
	if sess.agentState == StateConfirmPending {
		if sess.client != nil {
			sess.client.SendConfirm(true, false)
		}
		sess.agentState = StateToolExecuting
		return m, sess.thinkingAnim.Start()
	}

	if sess.agentState == StatePlanReview {
		text := strings.TrimSpace(sess.input.Value())
		action := "approve"
		if text != "" {
			action = "modify"
		}
		if sess.reconnecting {
			sess.pendingPlanAction = &pendingPlanAction{action: action, text: text}
			if text != "" {
				sess.input.Reset()
				sess.input.SetHeight(1)
			}
			return m, nil
		}
		if text == "" {
			if sess.client != nil {
				sess.client.SendPlanAction("approve", "")
			}
			sess.agentState = StateStreaming
		} else {
			sess.input.Reset()
			sess.input.SetHeight(1)
			if sess.client != nil {
				sess.client.SendPlanAction("modify", text)
			}
			sess.agentState = StateStreaming
		}
		return m, sess.thinkingAnim.Start()
	}

	if sess.agentState == StateStreaming || sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting {
		// Default Enter behavior: interrupt the current turn
		// AFTER the in-flight edit finishes, then send the new
		// message. queueOnly=false asks for a delayed cancel.
		return m.submitFromInput(sess, false)
	}

	if sess.agentState == StateWaitingForInput {
		text := strings.TrimSpace(sess.input.Value())
		if text == "" && sess.attachmentPanel.Count() == 0 {
			return m, nil
		}
		if text != "" {
			sess.history.Save(text)
		}
		sess.input.Reset()
		sess.input.SetHeight(1)

		panelAtts := sess.attachmentPanel.Clear()
		displayText, textAtts, imgErrs := extractImageAttachments(text)
		for _, e := range imgErrs {
			sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("%s", e)))
		}
		attachments := append(panelAtts, textAtts...)

		// Detect the first real user message in this session so we can
		// ask the default model to name the session. Only trigger when
		// the session has no label yet (it might already have one if it
		// was restored from disk or forked from a named session) and
		// when this is the first chat message that originated from the
		// user (system / status messages don't count).
		firstUser := sess.label == "" && isFirstUserMessage(sess.chatMessages)

		sess.chatMessages = append(sess.chatMessages, renderUserMessage(displayText, m.mdRenderer.width))
		sess.chatScrollOffset = 0

		sess.agentState = StateStreaming
		animCmd := sess.thinkingAnim.Start()

		if sess.client != nil {
			if sess.activeWorkflow != "" && !strings.HasPrefix(displayText, "/") {
				sess.client.SendWorkflow(sess.activeWorkflow, displayText)
			} else {
				sess.client.SendInput(displayText, attachments)
			}
		}
		if firstUser && !strings.HasPrefix(displayText, "/") {
			return m, tea.Batch(animCmd, generateSessionTitleCmd(sess.persistID, displayText))
		}
		return m, animCmd
	}
	return m, nil
}

// isFirstUserMessage reports whether the existing chat history contains
// isFirstUserMessage reports whether the existing chat history contains
// no user-typed messages yet. It walks the rendered messages looking
// for MsgUser entries; rendered system / error / status messages do not
// disqualify a session from being auto-named.
func isFirstUserMessage(msgs []ChatMessage) bool {
	for _, msg := range msgs {
		if msg.Type == MsgUser {
			return false
		}
	}
	return true
}
// applyEventToSession processes a single daemon event for the session at idx.
func (m *Model) applyEventToSession(idx int, event protocol.SessionEvent) []tea.Cmd {
	sess := m.sessions[idx]
	var cmds []tea.Cmd

	switch event.Type {
	case "event.session_started":
		data := marshalData(event.Data)
		var started protocol.EventSessionStarted
		json.Unmarshal(data, &started)
		sess.parentID = started.ParentID
		sess.forkTurnIdx = started.ForkTurnIdx

	case "event.init_state":
		data := marshalData(event.Data)
		var state protocol.EventInitState
		json.Unmarshal(data, &state)
		sess.initState = protocol.InitState(state.State)
		if state.Model != "" {
			sess.modelName = state.Model
		}

	case "event.workflows_available":
		data := marshalData(event.Data)
		var wa protocol.EventWorkflowsAvailable
		json.Unmarshal(data, &wa)
		sess.workflows = wa.Workflows
		if sess.activeWorkflow != "" {
			found := false
			for _, w := range sess.workflows {
				if w.Name == sess.activeWorkflow {
					found = true
					break
				}
			}
			if !found {
				sess.activeWorkflow = ""
			}
		}

	case "event.stream_chunk":
		data := marshalData(event.Data)
		var chunk protocol.EventStreamChunk
		json.Unmarshal(data, &chunk)
		sess.assistantBuf += chunk.Text
		sess.assistantRendered = m.mdRenderer.Render(sess.assistantBuf)

	case "event.thinking_chunk":
		data := marshalData(event.Data)
		var chunk protocol.EventThinkingChunk
		json.Unmarshal(data, &chunk)
		sess.thinkingBuf += chunk.Text
		if sess.showThinking {
			sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
		}

	case "event.stream_done":
		data := marshalData(event.Data)
		var done protocol.EventStreamDone
		json.Unmarshal(data, &done)
		sess.inputTokens += done.InputTokens
		sess.outputTokens += done.OutputTokens
		sess.cacheCreationTokens += done.CacheCreationTokens
		sess.cacheReadTokens += done.CacheReadTokens
		if done.ElapsedMs > 0 {
			sess.lastOutputTokens = done.OutputTokens
			sess.elapsed = time.Duration(done.ElapsedMs) * time.Millisecond
		}

	case "event.tool_call":
		m.flushSessionBuf(sess)
		sess.agentState = StateToolExecuting
		data := marshalData(event.Data)
		var tc protocol.EventToolCall
		json.Unmarshal(data, &tc)
		chatIdx := len(sess.chatMessages)
		sess.chatMessages = append(sess.chatMessages, renderToolCall(tc.Name, tc.Summary, tc.Reason,
			[4]string{tc.ReasonNotReadFile, tc.ReasonNotEditFile, tc.ReasonNotGlobFiles, tc.ReasonToIncreaseTimeout}, m.styles))
		if tc.ToolID != "" {
			if sess.pendingTools == nil {
				sess.pendingTools = make(map[string]int)
			}
			sess.pendingTools[tc.ToolID] = chatIdx
		}

	case "event.tool_result":
		data := marshalData(event.Data)
		var tr protocol.EventToolResult
		json.Unmarshal(data, &tr)
		detail := tr.Detail
		if sess.confirmDetailShown && tr.Name == sess.confirmToolName {
			detail = ""
			sess.confirmDetailShown = false
		}
		result := renderToolResultWithContext(tr.Name, tr.Output, tr.IsError, false, detail, m.styles, m.mdRenderer, m.mdRenderer.width)

		if tr.ToolID != "" && sess.pendingTools != nil {
			if callIdx, ok := sess.pendingTools[tr.ToolID]; ok {
				insertIdx := callIdx + 1
				delete(sess.pendingTools, tr.ToolID)
				if insertIdx <= len(sess.chatMessages) {
					sess.chatMessages = append(sess.chatMessages, ChatMessage{})
					copy(sess.chatMessages[insertIdx+1:], sess.chatMessages[insertIdx:])
					sess.chatMessages[insertIdx] = result
					for id, idx2 := range sess.pendingTools {
						if idx2 >= insertIdx {
							sess.pendingTools[id] = idx2 + 1
						}
					}
				} else {
					sess.chatMessages = append(sess.chatMessages, result)
				}
			} else {
				sess.chatMessages = append(sess.chatMessages, result)
			}
		} else {
			sess.chatMessages = append(sess.chatMessages, result)
		}

	case "event.confirm_request":
		sess.agentState = StateConfirmPending
		data := marshalData(event.Data)
		var cr protocol.EventConfirmRequest
		json.Unmarshal(data, &cr)
		sess.confirmToolName = cr.ToolName
		sess.confirmDetailShown = false
		sess.thinkingAnim.Stop()
		if cr.Detail != "" {
			sess.chatMessages = append(sess.chatMessages,
				renderToolResultWithContext(cr.ToolName, "", false, false, cr.Detail, m.styles, m.mdRenderer, m.mdRenderer.width))
			sess.confirmDetailShown = true
		}
		question := buildConfirmQuestion(cr.ToolName, cr.Params)
		if len(cr.RequestedDirs) > 0 {
			question = buildDirAccessQuestion(cr.RequestedDirs)
		}
		sess.chatMessages = append(sess.chatMessages,
			renderQuestionMessage("Permission", question, m.mdRenderer.width+4, m.mdRenderer))
		sess.questionPanel.OpenConfirm(cr.ToolName, cr.Params, cr.RequestedDirs, m.width, m.mdRenderer)
		sess.focus = FocusEditor

	case "event.user_question":
		data := marshalData(event.Data)
		var uq protocol.EventUserQuestion
		json.Unmarshal(data, &uq)
		sess.questionPanel.Open(uq, m.width, m.mdRenderer)
		sess.agentState = StateUserQuestion
		sess.thinkingAnim.Stop()
		sess.focus = FocusEditor
		sess.input.Blur()

	case "event.todo_list_updated":
		data := marshalData(event.Data)
		var tu protocol.EventTodoListUpdated
		json.Unmarshal(data, &tu)
		sess.todos = tu.Todos
		switch sess.rightPanel.mode {
		case rpModeWorkflow:
			// Todos render below workflow steps automatically.
		case rpModeTodos:
			if !hasPendingTodos(sess.todos) {
				sess.rightPanel.Close()
				m.updateChatWidth()
			}
		default:
			if !sess.rightPanel.IsVisible() && hasPendingTodos(sess.todos) {
				sess.rightPanel.OpenTodos(m.height)
				m.updateChatWidth()
			}
		}

	case "event.plan_proposed":
		data := marshalData(event.Data)
		var pp protocol.EventPlanProposed
		json.Unmarshal(data, &pp)
		sess.activePlan = pp.Plan
		sess.agentState = StatePlanReview
		sess.chatMessages = append(sess.chatMessages, renderPlanProposal(pp.Plan, m.styles))
		sess.input.Focus()
		sess.input.Placeholder = "Type modifications (Enter to send, Shift+Enter or Alt+Enter for new line) or press y/n..."

	case "event.plan_task_start":
		sess.agentState = StatePlanExecuting
		data := marshalData(event.Data)
		var pts protocol.EventPlanTaskStart
		json.Unmarshal(data, &pts)
		sess.chatMessages = append(sess.chatMessages, renderPlanTaskStart(pts.TaskIdx, pts.Title, pts.Total))
		cmds = append(cmds, sess.thinkingAnim.Start())

	case "event.plan_task_done":
		sess.thinkingAnim.Stop()
		data := marshalData(event.Data)
		var ptd protocol.EventPlanTaskDone
		json.Unmarshal(data, &ptd)
		sess.chatMessages = append(sess.chatMessages, renderPlanTaskDone(ptd.TaskIdx, ptd.Title, ptd.Success, ptd.Summary, m.styles))

	case "event.plan_complete":
		data := marshalData(event.Data)
		var pc protocol.EventPlanComplete
		json.Unmarshal(data, &pc)
		sess.activePlan = nil
		sess.chatMessages = append(sess.chatMessages, renderPlanSummary(pc.Plan))

	case "event.workflow_start":
		data := marshalData(event.Data)
		var ps protocol.EventWorkflowStart
		json.Unmarshal(data, &ps)
		sess.chatMessages = append(sess.chatMessages, renderWorkflowStart(ps.WorkflowName, ps.TotalSteps, m.styles))
		sess.workflowGraphPanel.Start(ps.WorkflowName, ps.TotalSteps, ps.Steps)
		sess.rightPanel.OpenWorkflow(m.height)
		m.updateChatWidth()

	case "event.workflow_step_start":
		sess.agentState = StateStreaming
		data := marshalData(event.Data)
		var pss protocol.EventWorkflowStepStart
		json.Unmarshal(data, &pss)
		sess.chatMessages = append(sess.chatMessages, renderWorkflowStepStart(pss.StepID, pss.StepIdx, pss.Total, pss.Explanation))
		sess.workflowGraphPanel.StepStart(pss.StepID, pss.StepIdx, pss.Explanation)
		cmds = append(cmds, sess.thinkingAnim.Start())

	case "event.workflow_step_done":
		sess.thinkingAnim.Stop()
		m.flushSessionBuf(sess)
		data := marshalData(event.Data)
		var psd protocol.EventWorkflowStepDone
		json.Unmarshal(data, &psd)
		sess.chatMessages = append(sess.chatMessages, renderWorkflowStepDone(psd.StepID, psd.StepIdx, psd.Total, psd.Success, psd.Display, psd.Command, psd.BashOutput, psd.ToolStats, m.mdRenderer, m.styles))
		sess.workflowGraphPanel.StepDone(psd.StepID, psd.Success, psd.DurationMs)

	case "event.workflow_complete":
		data := marshalData(event.Data)
		var pc protocol.EventWorkflowComplete
		json.Unmarshal(data, &pc)
		sess.chatMessages = append(sess.chatMessages, renderWorkflowComplete(pc.WorkflowName, pc.Success, pc.Summary, pc.StepCosts, pc.DurationMs, m.styles))
		sess.workflowGraphPanel.Reset()
		if hasPendingTodos(sess.todos) {
			sess.rightPanel.OpenTodos(m.height)
		} else {
			sess.rightPanel.Close()
			m.updateChatWidth()
		}

	case "event.agent_done":
		sess.thinkingAnim.Stop()
		m.flushSessionBuf(sess)
		if idx != m.selectedSession || m.activeTab != TabKindChat {
			sess.unreadCount++
		}
		turnInput := sess.inputTokens - sess.turnStartInputTokens
		turnOutput := sess.outputTokens - sess.turnStartOutputTokens
		turnCacheCreation := sess.cacheCreationTokens - sess.turnStartCacheCreationTokens
		turnCacheRead := sess.cacheReadTokens - sess.turnStartCacheReadTokens
		cost := protocol.CalculateCost(sess.modelName, turnInput, turnOutput, turnCacheCreation, turnCacheRead)
		sess.chatMessages = append(sess.chatMessages, renderTurnInfo(sess.modelName, sess.elapsed, cost, m.mdRenderer.width+4, m.styles))
		sess.turnStartInputTokens = sess.inputTokens
		sess.turnStartOutputTokens = sess.outputTokens
		sess.turnStartCacheCreationTokens = sess.cacheCreationTokens
		sess.turnStartCacheReadTokens = sess.cacheReadTokens
		// Persist chat scrollback to disk at the end of every turn
		// so an unexpected crash or kill does not lose the user's
		// most recent assistant response.
		m.persistSessions()
		if sess.pendingInput != nil {
			pending := sess.pendingInput
			sess.pendingInput = nil
			if sess.client != nil {
				if sess.activeWorkflow != "" && !strings.HasPrefix(pending.text, "/") {
					sess.client.SendWorkflow(sess.activeWorkflow, pending.text)
				} else {
					sess.client.SendInput(pending.text, pending.attachments)
				}
			}
			sess.agentState = StateStreaming
			// If another message is queued (e.g. the user
			// pressed Tab again while this turn was running),
			// keep showing the queued badge in the placeholder.
			sess.input.Placeholder = m.placeholderForMode(sess)
			cmds = append(cmds, sess.thinkingAnim.Start())
		} else {
			sess.agentState = StateWaitingForInput
			sess.input.Focus()
			sess.input.Placeholder = m.placeholderForMode(sess)
		}

	case "event.clear":
		m.flushSessionBuf(sess)
		sess.chatMessages = nil
		sess.pendingTools = nil
		sess.inputTokens = 0
		sess.outputTokens = 0
		sess.cacheCreationTokens = 0
		sess.cacheReadTokens = 0
		sess.turnStartInputTokens = 0
		sess.turnStartOutputTokens = 0
		sess.turnStartCacheCreationTokens = 0
		sess.turnStartCacheReadTokens = 0
		sess.elapsed = 0
		sess.chatMessages = append(sess.chatMessages, renderSystemMessage("Conversation cleared.", m.styles))
		// Persist the cleared scrollback.
		m.persistSessions()

	case "event.retry":
		data := marshalData(event.Data)
		var retry protocol.EventRetry
		json.Unmarshal(data, &retry)
		m.flushSessionBuf(sess)
		sess.chatMessages = append(sess.chatMessages, renderRetryMessage(retry))
		// Persist the retry notification.
		m.persistSessions()

	case "event.error":
		data := marshalData(event.Data)
		var errEvent protocol.EventError
		json.Unmarshal(data, &errEvent)
		sess.thinkingAnim.Stop()
		m.flushSessionBuf(sess)
		sess.pendingInput = nil
		sess.pendingPlanAction = nil
		sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("%s", errEvent.Message)))
		if sess.agentState != StatePlanReview && sess.agentState != StateUserQuestion && sess.agentState != StateConfirmPending {
			sess.agentState = StateWaitingForInput
			sess.input.Focus()
			sess.input.Placeholder = m.placeholderForMode(sess)
		}
		// Persist the latest scrollback so the error message is
		// on disk in case the user kills the process before any
		// further event fires.
		m.persistSessions()

	case "event.quit":
		cmds = append(cmds, tea.Quit)
	}

	return cmds
}

// View implements tea.Model — builds all content fresh each frame.
func (m Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	sess := m.currentSession()

	// Layout
	var panelHeights []int
	if sess != nil && sess.attachmentPanel.IsVisible() {
		panelHeights = append(panelHeights, sess.attachmentPanel.Count()+3)
	}
	if sess != nil && sess.historyPanel.IsVisible() {
		panelHeights = append(panelHeights, sess.historyPanel.maxHeight+2)
	}

	inputLines := m.visualLineCount()
	if sess != nil && (sess.agentState == StateUserQuestion || sess.agentState == StateConfirmPending) && sess.questionPanel.IsVisible() {
		inputLines = sess.questionPanel.Height()
	}
	layout := computeLayout(m.width, m.height, inputLines, panelHeights...)

	if sess != nil && sess.rightPanel.IsVisible() {
		layout.ChatWidth = m.width - sess.rightPanel.PanelWidth()
		if layout.ChatWidth < 10 {
			layout.ChatWidth = 10
		}
	}

	canvas := uv.NewScreenBuffer(m.width, m.height)
	screen.Clear(canvas)

	// When the user explicitly selected the "light" theme, force a
	// light background on the entire viewport. Without this, the
	// black text/border colors used by the light styles would be
	// invisible on terminals that have a dark default background
	// (which is almost all of them). Auto-detection of a light
	// terminal leaves the terminal's own background alone.
	if m.configuredTheme() == "light" {
		lightBG := &uv.Cell{
			Content: " ",
			Width:   1,
			Style:   uv.Style{Bg: lipgloss.Color("#FAFAFA")},
		}
		screen.FillArea(canvas, lightBG, image.Rect(0, 0, m.width, m.height))
	}

	y := 0

	// Tab bar
	viewportFocused := m.activeTab == TabKindSessions || m.activeTab == TabKindSettings || (sess != nil && sess.focus == FocusChat)
	tabBarWidth := layout.ChatWidth
	if m.activeTab == TabKindSessions || m.activeTab == TabKindSettings {
		tabBarWidth = m.width
	}
	tabBar := renderTabBar(m.activeTab, tabBarWidth, m.styles, viewportFocused, m.tabAlertBlinkOn)
	uv.NewStyledString(tabBar).Draw(canvas, image.Rect(0, y, tabBarWidth, y+layout.TabBarHeight))
	y += layout.TabBarHeight

	switch m.activeTab {
	case TabKindSessions:
		sessionsHeight := m.height - layout.TabBarHeight - layout.StatusBarHeight
		sv := renderSessionsView(m.sessions, m.width, sessionsHeight, m.styles, m.sessionsInput.Value(), m.sessionsInput.View(), m.sessionsSelected)
		uv.NewStyledString(sv).Draw(canvas, image.Rect(0, y, m.width, y+sessionsHeight))
		y += sessionsHeight

	case TabKindChat:
		// Chat content
		var chatContent string
		if sess != nil {
			chatContent = buildRenderedChat(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking)
			if sess.showThinking && sess.thinkingRendered != "" {
				chatContent += sess.thinkingRendered + "\n"
			}
			if sess.assistantRendered != "" {
				chatContent += sess.assistantRendered
			} else if animFrame := sess.thinkingAnim.View(); animFrame != "" {
				chatContent += animFrame + "\n"
			}
		}
		if chatContent == "" && !m.testMode {
			chatContent = renderWelcomeInline(m.mdRenderer.width, layout.ChatHeight-1, m.styles)
		}

		contentHeight := layout.ChatHeight - 1
		innerWidth := m.mdRenderer.width
		allLines := strings.Split(chatContent, "\n")

		visualRowStart := make([]int, len(allLines)+1)
		for i, line := range allLines {
			visualRowStart[i+1] = visualRowStart[i] + visualRows(line, innerWidth)
		}
		totalVisualRows := visualRowStart[len(allLines)]

		chatScrollOffset := 0
		if sess != nil {
			chatScrollOffset = sess.chatScrollOffset
		}
		endVisRow := totalVisualRows - chatScrollOffset
		if endVisRow < contentHeight {
			endVisRow = contentHeight
		}
		if endVisRow > totalVisualRows {
			endVisRow = totalVisualRows
		}

		endLogical := 0
		for endLogical < len(allLines) && visualRowStart[endLogical+1] <= endVisRow {
			endLogical++
		}
		accVisRows := 0
		startLogical := endLogical
		for startLogical > 0 {
			rows := visualRows(allLines[startLogical-1], innerWidth)
			if accVisRows+rows > contentHeight {
				break
			}
			accVisRows += rows
			startLogical--
		}

		chatLines := allLines[startLogical:endLogical]

		if sess != nil && sess.chatScrollOffset > 0 && sess.client != nil {
			for _, sep := range turnSeparatorInfos(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking) {
				if sep.LineIdx >= startLogical && sep.LineIdx < endLogical {
					chatLines[sep.LineIdx-startLogical] = renderForkHintLine(m.mdRenderer.width+4, m.styles)
					break
				}
			}
		}

		var chatBorderStyle lipgloss.Style
		if sess != nil && sess.focus == FocusChat {
			chatBorderStyle = m.styles.ViewportFocusedStyle
		} else if sess != nil && sess.focus == FocusRightPanel {
			chatBorderStyle = m.styles.ViewportBlurredStyle
		} else {
			chatBorderStyle = m.styles.ViewportBlurredStyle
		}
		chatBox := chatBorderStyle.Width(layout.ChatWidth).Height(layout.ChatHeight).
			Render(strings.Join(chatLines, "\n"))
		uv.NewStyledString(chatBox).Draw(canvas, image.Rect(0, y, layout.ChatWidth, y+layout.ChatHeight))

		// Right panel
		if sess != nil && sess.rightPanel.IsVisible() {
			rpHeight := layout.ChatHeight + 1
			rpView := sess.rightPanel.View(rpHeight, m.styles, sess.focus == FocusRightPanel, sess.modelName, &sess.workflowGraphPanel, sess.todos)
			rpX := m.width - sess.rightPanel.PanelWidth()
			uv.NewStyledString(rpView).Draw(canvas, image.Rect(rpX, y-1, m.width, y-1+rpHeight))
		}

		y += layout.ChatHeight

		// Panels between chat and input
		if sess != nil && sess.attachmentPanel.IsVisible() {
			panel := renderAttachmentPanel(&sess.attachmentPanel, m.width, m.styles)
			ph := sess.attachmentPanel.Count() + 3
			uv.NewStyledString(panel).Draw(canvas, image.Rect(0, y, m.width, y+ph))
			y += ph
		}
		if sess != nil && sess.historyPanel.IsVisible() {
			panel := renderHistoryPanel(sess.history.entries, sess.history.times, &sess.historyPanel, m.width, true, m.styles)
			ph := sess.historyPanel.maxHeight + 2
			uv.NewStyledString(panel).Draw(canvas, image.Rect(0, y, m.width, y+ph))
			y += ph
		}

		// Input section
		var inputSection string
		if sess != nil && (sess.agentState == StateUserQuestion || sess.agentState == StateConfirmPending) && sess.questionPanel.IsVisible() {
			inputSection = sess.questionPanel.Render(m.styles, sess.focus == FocusEditor, m.mdRenderer)
		} else if m.state == StateQuitConfirm {
			modeName := "Chat"
			if sess != nil {
				modeName = m.currentModeName(sess)
			}
			inputSection = renderInputBox(modeName, sess != nil && sess.activeWorkflow != "", "", m.width, false, m.styles.ColorBlurBorder)
		} else if sess != nil {
			inputSection = renderInputBox(m.currentModeName(sess), sess.activeWorkflow != "", sess.input.View(), m.width, sess.focus == FocusEditor, m.styles.ColorBlurBorder)
		} else {
			inputSection = renderInputBox("Chat", false, "", m.width, false, m.styles.ColorBlurBorder)
		}
		uv.NewStyledString(inputSection).Draw(canvas, image.Rect(0, y, m.width, y+layout.InputHeight))
		y += layout.InputHeight

	case TabKindSettings:
		settingsHeight := m.height - layout.TabBarHeight - layout.StatusBarHeight
		settingsActiveModel := m.currentSettingsModel()
		settingsActiveProvider := ProviderOfFromConfig(settingsActiveModel, m.cortexCfg)
		settingsActiveModel = m.canonicalSettingsModel(settingsActiveModel)
		settingsShowThinking := config.ShowThinking()
		if settSess := m.currentSession(); settSess != nil {
			settingsShowThinking = settSess.showThinking
		}
		providers := m.settingsProviders()
		selectedModels := m.selectedSettingsModels()
		otherView := SettingsOtherView{
			Theme:           m.configuredTheme(),
			ShowThinking:    settingsShowThinking,
			ReasoningEffort: m.currentReasoningEffort(),
			Streaming:       m.configuredStreaming(),
			ShowUsage:       m.configuredShowUsage(),
		}
		inspectView := m.settingsInspectView()
		wizardView := m.settingsWizardView()
		sv := renderSettingsView(m.width, settingsHeight, m.styles, m.settingsActiveSection, m.settingsProviderSel, m.settingsModelSel, m.settingsModelColumn, settingsActiveModel, settingsActiveProvider, providers, selectedModels, m.settingsKeys, m.settingsKeySel, m.settingsOtherSel, otherView, inspectView, m.settingsInKeyInput, m.settingsKeyInputLabel, m.settingsKeyInput.View(), wizardView)
		uv.NewStyledString(sv).Draw(canvas, image.Rect(0, y, m.width, y+settingsHeight))
		y += settingsHeight
	}
	// Status bar — global: connected if any session is up, reconnecting if none
	// are connected but at least one is trying.
	var connected, reconnecting bool
	for _, s := range m.sessions {
		if !s.reconnecting && s.client != nil {
			connected = true
			break
		}
		if s.reconnecting {
			reconnecting = true
		}
	}
	statusBar := renderStatusBar(m.width, connected, reconnecting, m.statusMsg, m.styles)
	uv.NewStyledString(statusBar).Draw(canvas, image.Rect(0, y, m.width, m.height))

	// Command palette overlay
	if m.commandPalette.IsVisible() {
		overlay := m.commandPalette.View(m.width, m.height, m.styles)
		w, h := lipgloss.Size(overlay)
		center := centerRect(canvas.Bounds(), w, h)
		uv.NewStyledString(overlay).Draw(canvas, center)
	}

	// Quit confirm overlay
	if m.state == StateQuitConfirm {
		overlay := renderQuitDialog(m.width, m.height, m.styles, m.quitSelected)
		w, h := lipgloss.Size(overlay)
		center := centerRect(canvas.Bounds(), w, h)
		uv.NewStyledString(overlay).Draw(canvas, center)
	}

	// Trim confirm overlay
	if sess != nil && sess.agentState == StateTrimConfirm {
		overlay := renderTrimDialog(m.width, m.height, m.styles, sess.trimSelected)
		w, h := lipgloss.Size(overlay)
		center := centerRect(canvas.Bounds(), w, h)
		uv.NewStyledString(overlay).Draw(canvas, center)
	}

	// Session close confirm overlay
	if m.state == StateSessionCloseConfirm {
		sessionID := ""
		if m.sessionCloseIdx >= 0 && m.sessionCloseIdx < len(m.sessions) {
			if s := m.sessions[m.sessionCloseIdx]; s.client != nil {
				sessionID = s.client.SessionID()
			}
		}
		overlay := renderSessionCloseDialog(m.width, m.height, m.styles, m.sessionCloseSelected, sessionID)
		w, h := lipgloss.Size(overlay)
		center := centerRect(canvas.Bounds(), w, h)
		uv.NewStyledString(overlay).Draw(canvas, center)
	}

	// File completer overlay
	if sess != nil && sess.fileCompleter.IsVisible() {
		popupWidth := 40
		if popupWidth > m.width-4 {
			popupWidth = m.width - 4
		}
		overlay := sess.fileCompleter.View(popupWidth, 8, m.styles)
		if overlay != "" {
			_, h := lipgloss.Size(overlay)
			inputTop := m.height - layout.StatusBarHeight - layout.InputHeight
			popupY := inputTop - h
			if popupY < 0 {
				popupY = 0
			}
			uv.NewStyledString(overlay).Draw(canvas, image.Rect(2, popupY, 2+popupWidth, popupY+h))
		}
	}

	// Slash menu overlay
	if sess != nil && sess.slashMenu.IsVisible() {
		overlay := sess.slashMenu.View(60, 8, m.styles)
		if overlay != "" {
			_, h := lipgloss.Size(overlay)
			inputTop := m.height - layout.StatusBarHeight - layout.InputHeight
			popupY := inputTop - h
			if popupY < 0 {
				popupY = 0
			}
			uv.NewStyledString(overlay).Draw(canvas, image.Rect(2, popupY, 2+60, popupY+h))
		}
	}

	content := strings.ReplaceAll(canvas.Render(), "\r\n", "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// --- Helper methods ---

// handleCommandAction executes the command identified by action and returns any
// resulting tea.Cmd values. It is shared by the command palette and slash menu.
func (m *Model) handleCommandAction(action string, sess *SessionState) []tea.Cmd {
	var cmds []tea.Cmd
	switch action {
	case "change_model":
		if sess != nil {
			sess.rightPanel.OpenModelSelect(m.height, sess.modelName)
			m.updateChatWidth()
			sess.focus = FocusRightPanel
			sess.input.Blur()
		}
	case "manage_keys":
		if sess != nil {
			sess.rightPanel.OpenKeyManager(m.height)
			m.updateChatWidth()
			sess.focus = FocusRightPanel
			sess.input.Blur()
		}
	case "clear":
		if sess != nil && sess.client != nil {
			sess.client.SendCancel()
		}
		if sess != nil {
			m.flushSessionBuf(sess)
			sess.chatMessages = nil
		}
	case "copy_conversation":
		if sess == nil || len(sess.chatMessages) == 0 {
			if sess != nil {
				sess.chatMessages = append(sess.chatMessages, renderSystemMessage("No conversation to copy.", m.styles))
			}
		} else {
			text := formatConversationPlainText(sess.chatMessages)
			count := len(sess.chatMessages)
			if err := clipboard.WriteAll(text); err != nil {
				sess.chatMessages = append(sess.chatMessages, renderErrorMessage(fmt.Errorf("failed to copy to clipboard: %w", err)))
			} else {
				sess.chatMessages = append(sess.chatMessages, renderSystemMessage(fmt.Sprintf("Copied %d messages to clipboard.", count), m.styles))
			}
		}
	case "slash_clear":
		if sess != nil && sess.client != nil {
			sess.chatMessages = append(sess.chatMessages, renderUserMessage("/clear", m.mdRenderer.width))
			sess.chatScrollOffset = 0
			sess.agentState = StateStreaming
			cmds = append(cmds, sess.thinkingAnim.Start())
			sess.client.SendInput("/clear", nil)
		}
	case "slash_skills":
		if sess != nil && sess.client != nil {
			sess.chatMessages = append(sess.chatMessages, renderUserMessage("/skills", m.mdRenderer.width))
			sess.chatScrollOffset = 0
			sess.agentState = StateStreaming
			cmds = append(cmds, sess.thinkingAnim.Start())
			sess.client.SendInput("/skills", nil)
		}
	case "history":
		if sess != nil && len(sess.history.entries) > 0 {
			sess.historyPanel.Open(len(sess.history.entries), m.height)
		}
	case "scroll_top":
		if sess != nil {
			sess.chatScrollOffset = m.sessionMaxScrollOffset(sess)
			sess.focus = FocusChat
		}
	case "scroll_bottom":
		if sess != nil {
			sess.chatScrollOffset = 0
			sess.focus = FocusChat
		}
	case "toggle_thinking":
		if sess != nil {
			sess.showThinking = !sess.showThinking
			if sess.showThinking && sess.thinkingBuf != "" {
				sess.thinkingRendered = renderThinkingText(sess.thinkingBuf, m.styles, m.mdRenderer.width+4)
			} else {
				sess.thinkingRendered = ""
			}
			_ = config.SetShowThinking(sess.showThinking)
		}
	case "quit":
		if sess != nil && sess.client != nil {
			sess.client.SendCancel()
			sess.client.SendClose()
		}
		// Persist chat scrollback on quit so the user does not lose
		// any in-flight messages.
		m.persistSessions()
		cmds = append(cmds, tea.Quit)
	default:
		if strings.HasPrefix(action, "switch_tab_") {
			idxStr := strings.TrimPrefix(action, "switch_tab_")
			if i, err := strconv.Atoi(idxStr); err == nil {
				switch TabKind(i) {
				case TabKindSessions:
					m.activeTab = TabKindSessions
					m.syncSessionsSelected()
					cmds = append(cmds, m.sessionsInput.Focus())
				case TabKindChat:
					m.activeTab = TabKindChat
					if sess != nil {
						sess.unreadCount = 0
						cmds = append(cmds, sess.thinkingAnim.Resume())
					}
				case TabKindSettings:
					m.openSettingsTab()
				}
			}
		}
	}
	return cmds
}

// flushSessionBuf commits the streaming assistant buffer to the session's chatMessages.
func (m *Model) flushSessionBuf(sess *SessionState) {
	if sess.showThinking && sess.thinkingBuf != "" {
		sess.chatMessages = append(sess.chatMessages, renderThinkingMessage(sess.thinkingBuf, m.styles, m.mdRenderer.width+4))
	}
	if sess.assistantBuf != "" {
		sess.chatMessages = append(sess.chatMessages, renderAssistantMessage(sess.assistantBuf, m.mdRenderer))
	}
	sess.assistantBuf = ""
	sess.assistantRendered = ""
	sess.thinkingBuf = ""
	sess.thinkingRendered = ""
}

// visualLineCount returns the display line count for the current session's input.
func (m *Model) visualLineCount() int {
	sess := m.currentSession()
	if sess == nil {
		return 1
	}
	val := sess.input.Value()
	if val == "" {
		return 1
	}
	availWidth := m.width - 4 - 2
	if availWidth <= 0 {
		return sess.input.LineCount()
	}
	total := 0
	for _, line := range strings.Split(val, "\n") {
		w := lipgloss.Width(line)
		total += w/availWidth + 1
	}
	if total < 1 {
		total = 1
	}
	if sess.input.MaxHeight > 0 && total > sess.input.MaxHeight {
		total = sess.input.MaxHeight
	}
	return total
}

// sessionMaxScrollOffset returns the max scroll offset for a session's chat.
func (m *Model) sessionMaxScrollOffset(sess *SessionState) int {
	layout := computeLayout(m.width, m.height, m.visualLineCount())
	contentHeight := layout.ChatHeight - 1
	chatContent := buildRenderedChat(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking)
	if sess.showThinking && sess.thinkingRendered != "" {
		chatContent += sess.thinkingRendered + "\n"
	}
	if sess.assistantRendered != "" {
		chatContent += sess.assistantRendered
	}
	if chatContent == "" && !m.testMode {
		chatContent = renderWelcomeInline(m.mdRenderer.width, contentHeight, m.styles)
	}
	innerWidth := m.mdRenderer.width
	totalVisualRows := 0
	for _, line := range strings.Split(chatContent, "\n") {
		totalVisualRows += visualRows(line, innerWidth)
	}
	maxOff := totalVisualRows - contentHeight
	if maxOff < 0 {
		return 0
	}
	return maxOff
}

// clampScrollOffset ensures the session's chatScrollOffset is within valid bounds.
func (m *Model) clampScrollOffset(sess *SessionState) {
	if sess.chatScrollOffset < 0 {
		sess.chatScrollOffset = 0
	}
	if max := m.sessionMaxScrollOffset(sess); sess.chatScrollOffset > max {
		sess.chatScrollOffset = max
	}
}

// sessionActiveForkSep returns the topmost visible turn separator when scrolled up.
func (m *Model) sessionActiveForkSep(sess *SessionState) (TurnSepInfo, bool) {
	if sess.chatScrollOffset == 0 || sess.client == nil {
		return TurnSepInfo{}, false
	}
	layout := computeLayout(m.width, m.height, m.visualLineCount())
	contentHeight := layout.ChatHeight - 1
	chatContent := buildRenderedChat(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking)
	if sess.showThinking && sess.thinkingRendered != "" {
		chatContent += sess.thinkingRendered + "\n"
	}
	if sess.assistantRendered != "" {
		chatContent += sess.assistantRendered
	}
	innerWidth := m.mdRenderer.width
	allLines := strings.Split(chatContent, "\n")
	visualRowStart := make([]int, len(allLines)+1)
	for i, line := range allLines {
		visualRowStart[i+1] = visualRowStart[i] + visualRows(line, innerWidth)
	}
	totalVisualRows := visualRowStart[len(allLines)]
	endVisRow := totalVisualRows - sess.chatScrollOffset
	if endVisRow < contentHeight {
		endVisRow = contentHeight
	}
	if endVisRow > totalVisualRows {
		endVisRow = totalVisualRows
	}
	endLogical := 0
	for endLogical < len(allLines) && visualRowStart[endLogical+1] <= endVisRow {
		endLogical++
	}
	accVisRows := 0
	startLogical := endLogical
	for startLogical > 0 {
		rows := visualRows(allLines[startLogical-1], innerWidth)
		if accVisRows+rows > contentHeight {
			break
		}
		accVisRows += rows
		startLogical--
	}
	for _, s := range turnSeparatorInfos(sess.chatMessages, m.styles, m.mdRenderer.width, sess.showThinking) {
		if s.LineIdx >= startLogical && s.LineIdx < endLogical {
			return s, true
		}
	}
	return TurnSepInfo{}, false
}

// doFork creates a new session seeded with history up to sep, and connects a fork.
func (m *Model) doFork(sep TurnSepInfo) (Model, tea.Cmd) {
	sess := m.currentSession()

	activeModel := m.activeModelForNewSession()
	newSess := newSessionState(m.cfg, nil)
	newSess.modelName = activeModel
	newSess.reconnecting = true
	forkedMsgs := make([]ChatMessage, sep.MsgIdx+1)
	copy(forkedMsgs, sess.chatMessages[:sep.MsgIdx+1])
	newSess.chatMessages = forkedMsgs

	forkSessionID := ""
	if sess.client != nil {
		forkSessionID = sess.client.SessionID()
	}

	newIdx := len(m.sessions)
	m.sessions = append(m.sessions, newSess)
	m.selectedSession = newIdx
	m.persistSessions()

	return *m, connectFork(
		m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken,
		m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess,
		forkSessionID, sep.TurnIdx, newSess.daemonSessionID,
	)
}

// doTrim trims the current session's history to sep and tells the daemon to match.
func (m *Model) doTrim(sep TurnSepInfo) (Model, tea.Cmd) {
	sess := m.currentSession()
	trimmed := make([]ChatMessage, sep.MsgIdx+1)
	copy(trimmed, sess.chatMessages[:sep.MsgIdx+1])
	sess.chatMessages = trimmed
	sess.chatScrollOffset = 0
	m.clampScrollOffset(sess)
	sess.agentState = sess.trimPrevState
	client := sess.client
	turnIdx := sep.TurnIdx
	cmd := func() tea.Msg {
		if client != nil {
			client.SendTrim(turnIdx)
		}
		return nil
	}
	return *m, cmd
}

// doCloseSession closes the session at sessionIdx and returns to the Sessions tab.
func (m *Model) doCloseSession(sessionIdx int) (Model, tea.Cmd) {
	if sessionIdx < 0 || sessionIdx >= len(m.sessions) {
		m.state = StateWaitingForInput
		return *m, nil
	}

	sess := m.sessions[sessionIdx]
	if sess.client != nil {
		sess.client.SendCancel()
		sess.client.SendClose()
	}

	m.sessions = append(m.sessions[:sessionIdx], m.sessions[sessionIdx+1:]...)

	if m.selectedSession >= len(m.sessions) {
		m.selectedSession = len(m.sessions) - 1
	}
	if m.selectedSession < 0 {
		m.selectedSession = 0
	}

	var reconnectCmd tea.Cmd
	if len(m.sessions) == 0 {
		activeModel := m.activeModelForNewSession()
		newSess := newSessionState(m.cfg, nil)
		newSess.modelName = activeModel
		newSess.reconnecting = true
		m.sessions = append(m.sessions, newSess)
		m.selectedSession = 0
		reconnectCmd = attemptReconnect(m.socketPath, m.cwd, m.cfg.ConfigDir, activeModel, m.authToken, false, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess, newSess.daemonSessionID)
	}

	if n := m.sessionsVisibleCount(); n > 0 && m.sessionsSelected >= n {
		m.sessionsSelected = n - 1
	}

	m.activeTab = TabKindSessions
	m.syncSessionsSelected()
	m.state = StateWaitingForInput
	m.persistSessions()
	return *m, reconnectCmd
}

// updateChatWidth updates the markdown renderer width to match the current effective chat width.
func (m *Model) updateChatWidth() {
	sess := m.currentSession()
	chatWidth := computeLayout(m.width, m.height, m.visualLineCount()).ChatWidth
	if sess != nil && sess.rightPanel.IsVisible() {
		chatWidth = m.width - sess.rightPanel.PanelWidth()
		if chatWidth < 10 {
			chatWidth = 10
		}
	}
	m.mdRenderer.UpdateWidth(chatWidth - 4)
	m.rerenderSessionMessages()
}

// rerenderSessionMessages re-renders the current session's chat messages at the current width.
func (m *Model) rerenderSessionMessages() {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	width := m.mdRenderer.width + 4
	for i, msg := range sess.chatMessages {
		sess.chatMessages[i] = msg.rerender(m.mdRenderer, m.styles, width)
	}
}

// visibleSessionIndices returns the indices of sessions that match the current filter.
func (m *Model) visibleSessionIndices() []int {
	const colSession = 10
	const colMessage = 52
	filterLower := strings.ToLower(m.sessionsInput.Value())
	var indices []int
	for i, sess := range m.sessions {
		if filterLower == "" {
			indices = append(indices, i)
			continue
		}
		sessionCol := "connecting…"
		if sess.client != nil {
			id := sess.client.SessionID()
			if dash := strings.Index(id, "-"); dash >= 0 {
				sessionCol = id[:dash]
			} else if len(id) > colSession {
				sessionCol = id[:colSession]
			} else {
				sessionCol = id
			}
		}
		msgCol := "—"
		for _, msg := range sess.chatMessages {
			if msg.Type == MsgUser {
				line := strings.SplitN(msg.Text, "\n", 2)[0]
				if len(line) > colMessage {
					line = line[:colMessage-1] + "…"
				}
				msgCol = line
				break
			}
		}
		if strings.Contains(strings.ToLower(sessionCol), filterLower) ||
			strings.Contains(strings.ToLower(msgCol), filterLower) {
			indices = append(indices, i)
		}
	}
	return indices
}

// sessionsVisibleCount returns the number of visible sessions (after filter).
func (m *Model) sessionsVisibleCount() int {
	return len(m.visibleSessionIndices())
}

// syncSessionsSelected sets sessionsSelected to the visible row that corresponds
// to the currently active workspace session (selectedSession).
func (m *Model) syncSessionsSelected() {
	for i, idx := range m.visibleSessionIndices() {
		if idx == m.selectedSession {
			m.sessionsSelected = i
			return
		}
	}
}

// sessionsSelectedIdx returns the session index for the highlighted row.
func (m *Model) sessionsSelectedIdx() (int, bool) {
	indices := m.visibleSessionIndices()
	if m.sessionsSelected < 0 || m.sessionsSelected >= len(indices) {
		return 0, false
	}
	return indices[m.sessionsSelected], true
}

// hasAlertSessions reports whether any session is waiting for user input.
func (m *Model) hasAlertSessions() bool {
	for _, sess := range m.sessions {
		if sess.agentState == StateConfirmPending || sess.agentState == StateUserQuestion {
			return true
		}
	}
	return false
}

// maybeStartTabAlertBlink starts the tab alert blink if any session needs attention.
func (m *Model) maybeStartTabAlertBlink() tea.Cmd {
	if m.tabAlertActive || !m.hasAlertSessions() {
		return nil
	}
	m.tabAlertActive = true
	m.tabAlertBlinkOn = true
	return m.tabBlinkTick()
}

// stopTabAlertBlink halts the blink loop.
func (m *Model) stopTabAlertBlink() {
	m.tabAlertActive = false
	m.tabAlertBlinkOn = false
	m.tabAlertBlinkGen++
}

// tabBlinkTick schedules the next tab blink toggle.
func (m *Model) tabBlinkTick() tea.Cmd {
	gen := m.tabAlertBlinkGen
	return tea.Tick(tabBlinkHalfPeriod, func(time.Time) tea.Msg {
		return tabBlinkMsg{gen: gen}
	})
}

// nextWorkflow cycles through available workflows for a session.
func (m *Model) nextWorkflow(sess *SessionState) string {
	if sess.activeWorkflow == "" {
		if len(sess.workflows) > 0 {
			return sess.workflows[0].Name
		}
		return ""
	}
	for i, w := range sess.workflows {
		if w.Name == sess.activeWorkflow {
			if i+1 < len(sess.workflows) {
				return sess.workflows[i+1].Name
			}
			return ""
		}
	}
	return ""
}

// currentModeName returns "Chat" or the active workflow name.
func (m *Model) currentModeName(sess *SessionState) string {
	if sess.activeWorkflow == "" {
		return "Chat"
	}
	for _, w := range sess.workflows {
		if w.Name == sess.activeWorkflow {
			return w.Name
		}
	}
	return "Chat"
}

// emitStatusMsg sets the global transient status bar message and returns a
// tea.Cmd that clears it after 3 seconds. Rapid successive calls are safe
// because each call bumps the generation counter; only the matching clear fires.
func (m *Model) emitStatusMsg(text string, kind StatusMsgKind) tea.Cmd {
	m.statusMsg.gen++
	m.statusMsg.Text = text
	m.statusMsg.Kind = kind
	gen := m.statusMsg.gen
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return clearStatusMsgMsg{gen: gen}
	})
}

// placeholderForMode returns mode-specific placeholder text.
// When a message is queued (pendingInput is non-nil) the
// placeholder includes a small badge so the user can see at a
// glance that something is waiting to be sent and whether it was
// queued with Tab (no cancel) or Enter (delayed cancel).
func (m *Model) placeholderForMode(sess *SessionState) string {
	if sess.pendingInput != nil {
		preview := sess.pendingInput.text
		if len(preview) > 40 {
			preview = preview[:37] + "…"
		}
		preview = strings.ReplaceAll(preview, "\n", " ")
		if sess.pendingInput.Queued {
			return fmt.Sprintf("⏳ Queued: %q — Tab queued, Enter to interrupt and send now", preview)
		}
		return fmt.Sprintf("⏳ Sending after current edit: %q", preview)
	}
	if sess.activeWorkflow == "" {
		return "Ask the agent anything... (Enter to send, Shift+Enter or Alt+Enter for new line)"
	}
	for _, w := range sess.workflows {
		if w.Name == sess.activeWorkflow {
			return fmt.Sprintf("Describe your %s... (Enter to send, Shift+Enter or Alt+Enter for new line)", w.Name)
		}
	}
	return "Enter your request... (Enter to send, Shift+Enter or Alt+Enter for new line)"
}

// updateInputPromptColor sets the textarea text style to match the current mode.
func (m *Model) updateInputPromptColor(sess *SessionState) {
	whiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	s := sess.input.Styles()
	s.Focused.Text = whiteStyle
	s.Focused.CursorLine = whiteStyle
	s.Blurred.Text = lipgloss.NewStyle().Foreground(colorDim)
	sess.input.SetStyles(s)
}

// marshalData converts event.Data back to bytes.
func marshalData(data any) []byte {
	b, _ := json.Marshal(data)
	return b
}

// fillTestData populates the current session with fake messages for UI testing.
func (m *Model) fillTestData() {
	sess := m.currentSession()
	if sess == nil {
		return
	}
	sess.chatMessages = append(sess.chatMessages,
		renderSystemMessage("Test mode -- fake data for UI scroll testing", m.styles),
		renderUserMessage("Can you help me refactor the authentication module?", m.mdRenderer.width),
		renderAssistantMessage("Sure! Let me start by reading the current auth implementation.", m.mdRenderer),
		renderToolCall("read_file", "internal/auth/handler.go", "", [4]string{}, m.styles),
		renderToolResult("read_file", "package auth\n\n// handler code...", false, m.styles, m.mdRenderer, m.mdRenderer.width),
		renderAssistantMessage("I can see the auth module. Here's what I'd suggest for the refactor.", m.mdRenderer),
	)
}
