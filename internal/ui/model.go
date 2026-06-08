package ui

import (
	"context"
	"fmt"
	"image"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/ultraviolet/screen"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider/codex"
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
	Text    string
	Kind    StatusMsgKind
	gen     int // stale-clear guard
	Spinner int // spinner frame index (0..7); -1 = no spinner
}


// clearStatusMsgMsg clears the status bar message when its generation matches.
type clearStatusMsgMsg struct{ gen int }

// startCursorBlinkMsg triggers cursor blink on startup.
type startCursorBlinkMsg struct{}

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
	settingsActiveSection    int    // 0=providers, 1=other settings (Tab toggles between them)
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

	// compactInFlight is true while a /compact (or auto-compact)
	// run is in progress. The flag prevents stacking multiple
	// compactions when several turns complete in quick
	// succession (each one would otherwise see the same 80%+ usage
	// and fire another compaction).
	compactInFlight bool

	// updateProgress is the latest step name from the
	// /update self-update flow. It's a sync/atomic.Value
	// string so the progress tick goroutine can write it
	// without locking. The status bar reads it on every
	// frame and shows the value as a transient message.
	updateProgress atomic.Value // string
	// updateProgressFrame cycles 0..7 every progress tick
	// so the status bar can show a spinner (⠋⠙⠹⠸⠼⠴⠦⠧)
	// animating alongside the step name. Without it the
	// message just sat there as a static "Checking for
	// updates…" string and the user reported "/update
	// doesn't do the animation".
	updateProgressFrame int
	// updateOverlay drives the big "Updating cortex"
	// modal that replaces the previous status-bar-only
	// progress. The user reported: "the /update animation
	// should show a big pop up with a cool animation! and
	// then the cli should restart once its ready and the
	// tui has been updated". See update_overlay.go for
	// the rendering and phase machine.
	updateOverlay updateOverlayState
	// codexAuthPending is true while the codex OAuth flow
	// is in flight. The View() shows a full-screen
	// "waiting for auth" overlay with the authorize URL
	// so the user can copy it into a browser manually if
	// the auto-open fails.
	codexAuthPending   bool
	codexAuthURL       string
	codexAuthModel     string
	codexAuthStartedAt time.Time

	// Shared rendering
	mdRenderer     *MarkdownRenderer
	commandPalette CommandPalette
	modelPicker    ModelPicker
	loginPicker    LoginPicker

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

	// Mouse / clickable UI support.
	// We record the latest cursor position on motion and clicks,
	// and use simple hit-testing ("cursor detection") in Update to
	// turn clicks into actions (e.g. switching tabs by clicking the
	// tab bar, or in the future: clicking model entries, links, etc.).
	mouseX, mouseY  int
	lastClickX      int
	lastClickY      int
	lastClickButton tea.MouseButton

	mouseButtonDown bool
	mouseHover        mouseHover

	// Right-click context menu (paste/copy) for Linux terminals
	// where the emulator menu is blocked by mouse capture.
	contextMenu contextMenu
}

// currentSession returns the selected session, or nil if there is none.
func (m *Model) currentSession() *SessionState {
	if m.selectedSession < 0 || m.selectedSession >= len(m.sessions) {
		return nil
	}
	return m.sessions[m.selectedSession]
}

// buildStatusBarInfo projects the slim-footer readouts from the
// current session state. The footer shows: model, ctx%, elapsed.
// If the session is nil (no chat yet) the values are zeroed and
// the footer degrades to just the connection status.
func (m *Model) buildStatusBarInfo(sess *SessionState) StatusBarInfo {
	info := StatusBarInfo{
		AutoCompact: m.configuredAutoCompact(),
	}
	if sess == nil {
		return info
	}
	info.InputTokens = sess.inputTokens
	info.CacheRead = sess.cacheReadTokens
	info.Elapsed = sess.TurnElapsed()
	if sess.pendingInput != nil && sess.pendingInput.Queued {
		// A single pending message can be queued (Tab)
		// waiting for the current turn to finish. Surface
		// it as "1 queued" in the status bar so the user
		// knows they have something waiting.
		info.QueuedMsgs = 1
	}
	if spec := m.currentSettingsModel(); spec != "" {
		info.ModelName = m.displayNameForModelSpec(spec)
		if colon := strings.Index(spec, ":"); colon > 0 {
			info.ProviderTag = spec[:colon]
		}
	}
	if max := cortexconfig.ModelContextWindow(m.currentSettingsModel()); max > 0 {
		info.ContextMax = max
	}
	// Fallback to chars/4 estimate so the bar shows something
	// even before the first turn completes.
	if info.InputTokens == 0 {
		chars := 0
		for _, msg := range sess.chatMessages {
			chars += len(msg.Text)
		}
		if chars > 0 {
			info.InputTokens = int64(chars / 4)
		}
	}
	return info
}

// buildRightPanelInfoView collects the data the right panel's
// info / status mode needs from the live model state. Keeping
// this in the model layer (rather than the view layer) means the
// right panel never has to know about SessionState / config /
// timing, and tests can build an info view by hand.
func (m *Model) buildRightPanelInfoView(sess *SessionState) RightPanelInfoView {
	info := RightPanelInfoView{
		SessionCount: len(m.sessions),
		AutoCompact:  m.configuredAutoCompact(),
	}
	if sess != nil {
		info.InputTokens = sess.inputTokens
		info.OutputTokens = sess.outputTokens
		info.CacheRead = sess.cacheReadTokens
		info.Elapsed = sess.TurnElapsed()
		// Surface the AI's todo list in the right panel so
		// the user can see what the agent is working on
		// (the user reported 'the AI never makes a todo
		// list when asked' — that was because nothing was
		// rendering in the right panel).
		info.Todos = sess.todos
		if sess.pendingInput != nil && sess.pendingInput.Queued {
			info.QueuedMsgs = 1
		}
		if sess.client != nil {
			// SessionClient has no public Connected()
			// method, so we treat non-nil + not currently
			// reconnecting as "connected". This matches
			// the status-bar connection check a few
			// hundred lines down.
			info.Connected = !sess.reconnecting
		}
		if sess.modelName != "" {
			info.ModelName = sess.modelName
		}
		// Look up the provider display name from the
		// cortexconfig presets so the panel can show
		// "ChatGPT (codex)" instead of just "codex".
		if m.cortexCfg != nil {
			// First try: currentSettingsModel() returns
			// the spec the user picked via /model; if
			// that has a friendly name use it.
			if spec := m.currentSettingsModel(); spec != "" {
				if display := m.displayNameForModelSpec(spec); display != "" {
					info.ModelName = display
				}
			}
			// Resolve "provider" by stripping the
			// "<provider>:" prefix from the spec.
			if spec := m.currentSettingsModel(); spec != "" {
				if colon := strings.Index(spec, ":"); colon > 0 {
					info.ProviderName = cortexconfig.ProviderDisplayName(spec[:colon])
				}
			}
		}
	}
	// Look up the model's context window.
	if m.cortexCfg != nil {
		if max := cortexconfig.ModelContextWindow(m.currentSettingsModel()); max > 0 {
			info.ContextMax = max
		}
	}
	// Fallback to the configured model's known window so the
	// bar always shows a percentage (even for the default
	// model the user hasn't picked via /model).
	if info.ContextMax == 0 && m.cortexCfg != nil && m.cortexCfg.DefaultModel != "" {
		if max := cortexconfig.ModelContextWindow(m.cortexCfg.DefaultModel); max > 0 {
			info.ContextMax = max
		}
	}
	// Final fallback: if we still have no context window
	// (custom / local / unknown model), assume 200k so the
	// user always sees a meaningful "12k / 200k (6%)" line
	// in the right panel and slim footer instead of a
	// permanent "ctx 12k" with no denominator.
	if info.ContextMax == 0 {
		info.ContextMax = 200_000
	}
	// If we still don't have a real token count, fall back to a
	// chars/4 estimate across the chat history so the bar isn't
	// permanently at 0% on a brand-new session.
	if info.InputTokens == 0 && sess != nil {
		chars := 0
		for _, msg := range sess.chatMessages {
			chars += len(msg.Text)
		}
		if chars > 0 {
			info.InputTokens = int64(chars / 4)
		}
	}
	// If we still don't have a model name, fall back to the
	// configured default. (Helps when the session hasn't
	// resolved its model yet.)
	if info.ModelName == "" && m.cortexCfg != nil {
		info.ModelName = m.cortexCfg.DefaultModel
	}
	return info
}

// startCodexLoginCmd kicks off the codex OAuth flow in the background.
// The returned tea.Cmd resolves to a codexLoginStartedMsg
// (with the authorize URL) immediately, then to a
// codexLoginSuccessMsg or codexLoginFailedMsg after the browser
// round-trip. The UI listens for the started msg to show a
// large "waiting for auth" overlay with the URL — this is the
// big pop-up the user asked for, in case the browser doesn't
// auto-open (headless mode, WSL, SSH) the user can copy the
// URL and paste it in any browser.
func (m *Model) startCodexLoginCmd(pendingModel string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			// The login itself happens here. We need the URL
			// before we kick off the round-trip so the UI
			// can show it; we pre-build it via codex.AuthURL().
			authURL := codex.AuthURL()
			return codexLoginStartedMsg{pendingModel: pendingModel, authorizeURL: authURL}
		},
		func() tea.Msg {
			// Give the user a 5-minute window to complete the OAuth flow.
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
		},
	)
}

// startCodexDeviceLoginCmd is the device-code fallback for the
// codex OAuth flow. Instead of opening a browser and waiting for
// a localhost callback, it asks OpenAI for a one-time user_code,
// prints the verification URL + code, and polls for completion.
// This is the right flow when:
//
//   - the user is on an SSH / WSL / cloud machine where
//     localhost:1455 isn't reachable from their browser;
//   - the user's account triggers the "add phone number" gate
//     on the localhost-callback flow (the auth server returns
//     "Invalid authorize request" — see
//     https://github.com/openai/codex/issues/20161).
//
// The user must look at the TUI for the user_code, then go to
// https://auth.openai.com/codex/device in any browser, sign in,
// and paste the code. Once they do, the poll completes and the
// TUI shows the same "Switched to codex/gpt-5.5" status as the
// browser flow.
func (m *Model) startCodexDeviceLoginCmd(pendingModel string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
		defer cancel()
		res, err := codex.DeviceLogin(ctx)
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

// startCodexDeviceLoginPromptCmd is a wrapper that first requests
// the user_code (so the user sees it in the TUI) and then kicks
// off the poll. We split this from startCodexDeviceLoginCmd so
// the UI can show the prompt with a "Waiting for you to enter
// the code at https://auth.openai.com/codex/device ..." status
// line. Returns a two-step cmd: first emits a
// codexDeviceCodeReadyMsg carrying the user_code + URL, then runs
// the poll that eventually resolves to a codexLoginSuccessMsg
// or codexLoginFailedMsg.
func (m *Model) startCodexDeviceLoginPromptCmd(pendingModel string) tea.Cmd {
	cmds := []tea.Cmd{}
	// Show the prompt first. We don't have the user_code yet —
	// we have to ask OpenAI for it. The whole flow is
	// synchronous so we can't return the prompt before the
	// first network request; instead we kick off the whole
	// thing in one cmd and the user sees "Opening codex
	// sign-in in your browser…" in the meantime. The result of
	// the poll resolves to codexLoginSuccessMsg or
	// codexLoginFailedMsg exactly like the browser flow.
	cmds = append(cmds, m.emitStatusMsg(
		"Requesting ChatGPT (codex) one-time code (15-min window)…",
		StatusMsgInfo,
	))
	cmds = append(cmds, m.startCodexDeviceLoginCmd(pendingModel))
	return tea.Batch(cmds...)
}

// startOAuthLoginCmd is the generic dispatcher for OAuth providers.
// It only knows how to actually launch the codex flow today; other
// OAuth providers (claude-sub, copilot) currently take their token
// from an env var and don't need a flow. If a future refactor adds
// real OAuth flows for those, switch on providerName here.
func (m *Model) startOAuthLoginCmd(providerName string) tea.Cmd {
	switch providerName {
	case "codex":
		return m.startCodexLoginCmd("")
	case "claude-sub":
		return m.emitStatusMsg("Claude Pro/Max: set CLAUDE_CODE_OAUTH_TOKEN=<token> in your environment, then restart cortex-cli", StatusMsgInfo)
	case "copilot":
		return m.emitStatusMsg("GitHub Copilot: set COPILOT_OAUTH_TOKEN=<token> in your environment, then restart cortex-cli", StatusMsgInfo)
	default:
		return m.emitStatusMsg("OAuth flow not implemented for "+providerName+"; set the relevant env var", StatusMsgError)
	}
}

// compactProgressTick fires periodically while compaction is
// in-flight so the status bar spinner animates. 120ms matches
// the self-update spinner cadence.
func compactProgressTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return compactProgressMsg{}
	})
}

// compactProgressMsg fires periodically while a /compact run is
// in progress. The handler advances the status bar spinner frame.
type compactProgressMsg struct{}

// autosaveTickMsg periodically persists sessions
// while the TUI is running. This protects users from
// losing recent chat state when cortex crashes or the
// terminal/process is killed. Empty sessions are still
// filtered by persistSessions(), so autosave won't
// bloat the Sessions tab.
type autosaveTickMsg struct{}

func autosaveTick() tea.Cmd {
	return tea.Tick(20*time.Second, func(time.Time) tea.Msg {
		return autosaveTickMsg{}
	})
}

// applyModelPickerSelection routes a spec chosen from the /model
// picker to the right action. OAuth providers (codex, claude-sub,
// copilot) fire the OAuth flow directly — the user never sees the
// "enter API key" form. API-key providers that already have a key
// stored switch models immediately. API-key providers without a
// key fall through to the right-panel key input form so the user
// can paste one. Local / env-var providers switch models
// immediately (no key needed).
func (m *Model) applyModelPickerSelection(spec string) tea.Cmd {
	if spec == "" {
		return nil
	}
	provider, _, _ := cortexconfig.SplitModelSpec(spec)
	if provider == "" {
		provider = spec
	}
	provider = cortexconfig.NormalizeProviderName(provider)
	authKind := cortexconfig.ProviderAuthKind(provider)

	// OAuth providers: fire the browser flow. The user does NOT
	// see any key/base-URL form.
	if authKind == "oauth" {
		if provider == "codex" {
			return tea.Batch(
				m.emitStatusMsg("Opening ChatGPT (codex) sign-in in your browser\u2026", StatusMsgInfo),
				m.startCodexLoginCmd(spec),
			)
		}
		// claude-sub / copilot: no browser flow yet; tell the
		// user which env var to set.
		var envVar, displayName string
		switch provider {
		case "claude-sub":
			envVar = "CLAUDE_CODE_OAUTH_TOKEN"
			displayName = "Claude Pro/Max"
		case "copilot":
			envVar = "COPILOT_OAUTH_TOKEN"
			displayName = "GitHub Copilot"
		default:
			envVar = "<env var>"
			displayName = provider
		}
		return m.emitStatusMsg(displayName+": set "+envVar+"=<token> in your environment, then restart cortex-cli.", StatusMsgInfo)
	}

	// Local / env-var providers: no key to collect, just switch.
	if authKind == "none" || authKind == "env" {
		m.setActiveModelSpec(spec)
		if m.cortexCfg != nil {
			m.cortexCfg.DefaultModel = spec
			_ = cortexconfig.Save(m.cortexCfg)
		}
		sess := m.currentSession()
		if sess != nil && sess.client != nil {
			_ = sess.client.SendSetModel(spec)
		}
		return m.emitStatusMsg("Switched to "+spec, StatusMsgInfo)
	}

	// API-key providers: ensure the provider row exists, then
	// check if a key is already stored. If yes, switch
	// immediately. If no, drop the user into the right-panel
	// key-input form so they can paste one.
	if m.cortexCfg != nil {
		_, model, _ := cortexconfig.SplitModelSpec(spec)
		if ensured := m.cortexCfg.EnsureProviderModel(provider, model); ensured != "" {
			spec = ensured
		}
	}
	if key, _ := config.ResolveProviderKey(provider, false); key != "" {
		m.setActiveModelSpec(spec)
		if m.cortexCfg != nil {
			m.cortexCfg.DefaultModel = spec
			_ = cortexconfig.Save(m.cortexCfg)
		}
		sess := m.currentSession()
		if sess != nil && sess.client != nil {
			_ = sess.client.SendSetModel(spec)
		}
		return m.emitStatusMsg("Switched to "+spec, StatusMsgInfo)
	}
	// No key stored — open the right-panel key input form
	// (which is the only place we ask for an API key now that
	// the Settings wizard is gone).
	sess := m.currentSession()
	if sess != nil {
		sess.rightPanel.OpenKeyInput(provider, spec, m.height)
		m.updateChatWidth()
		sess.focus = FocusRightPanel
		sess.input.Blur()
	}
	return m.emitStatusMsg("API key needed for "+provider+" — paste it in the right panel", StatusMsgInfo)
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

// workflowEventMsg is the internal hook callback the engine
// uses to notify the UI. We funnel everything through a
// tea.Msg so the Update goroutine can react without holding
// locks.
//
// `kind` is one of "start", "step_start", "step_done",
// "step_progress:<msg>", or "complete". `stepID` is the
// affected step (empty for workflow-level events).
// handleCodexLoginFailed surfaces the failure in the status bar.
// If the browser couldn't be opened, include the authorize URL so the
// user can copy it from the status message and paste into a browser
// manually (e.g. on a headless server).
func (m *Model) handleCodexLoginFailed(err error, authorizeURL string) tea.Cmd {
	msg := "ChatGPT sign-in failed: " + err.Error()
	if authorizeURL != "" {
		msg += " — open " + authorizeURL + " manually"
	}
	// If the auth server bounced us with the "Invalid authorize
	// request" / "add phone number" gate, give the user a
	// pointer to the device-code fallback so they're not stuck.
	if strings.Contains(err.Error(), "Invalid authorize") ||
		strings.Contains(err.Error(), "add phone") ||
		strings.Contains(err.Error(), "phone number") {
		msg += " — if the browser shows a phone-verification gate, " +
			"type /login codex --device to use the device-code flow " +
			"instead (works on SSH/WSL and accounts without a phone on file)"
	}
	return m.emitStatusMsg(msg, StatusMsgError)
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
		testMode: testMode,
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
	cmds = append(cmds, waitForResume, tea.RequestBackgroundColor, autosaveTick())
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		m.noteMousePosition(mouse.X, mouse.Y)
		m.updateMouseHover(mouse.X, mouse.Y)
		m.lastClickX = m.mouseX
		m.lastClickY = m.mouseY
		m.lastClickButton = msg.Button

		if m.contextMenu.active {
			if msg.Button == tea.MouseLeft {
				return m.handleContextMenuClick(mouse.X, mouse.Y)
			}
			m.closeContextMenu()
			return m, nil
		}

		if msg.Button == tea.MouseLeft {
			m.mouseButtonDown = true
		}

		if m.mouseInTabBar(m.mouseY) {
			var cmd tea.Cmd
			m, cmd = m.handleTabBarClick()
			return m, cmd
		}

		if msg.Button == tea.MouseRight && m.canOpenContextMenu() {
			m.openContextMenu(mouse.X, mouse.Y, m.buildContextMenuItems())
			return m, nil
		}

		if m.activeTab == TabKindChat && msg.Button == tea.MouseLeft {
			m.handleChatMouseDown(mouse.X, mouse.Y)
		}

	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		m.noteMousePosition(mouse.X, mouse.Y)
		m.mouseButtonDown = mouse.Button == tea.MouseLeft
		m.updateMouseHover(mouse.X, mouse.Y)
		if m.activeTab == TabKindChat && m.mouseButtonDown {
			m.handleChatMouseDrag(mouse.X, mouse.Y)
		}
		return m, nil

	case tea.MouseWheelMsg:
		// Mouse wheel always scrolls the chat content when the Chat
		// tab is active, regardless of keyboard focus or side panels.
		// This is a primary way to scroll "no matter where you are".
		m.noteMousePosition(msg.Mouse().X, msg.Mouse().Y)
		sess := m.currentSession()
		if m.activeTab == TabKindChat && sess != nil {
			delta := 5
			switch msg.Button {
			case tea.MouseWheelUp:
				sess.chatScrollOffset += delta
			case tea.MouseWheelDown:
				sess.chatScrollOffset -= delta
			}
			m.clampScrollOffset(sess)
			return m, nil
		}
		return m, nil

	case tea.MouseReleaseMsg:
		// Also handle on release for reliability in some terminals.
		mouse := msg.Mouse()
		m.noteMousePosition(mouse.X, mouse.Y)
		m.lastClickX = m.mouseX
		m.lastClickY = m.mouseY
		m.lastClickButton = msg.Button
		m.mouseButtonDown = false

		if m.activeTab == TabKindChat {
			m.extendChatSelection(mouse.X, mouse.Y)
		}

		if m.mouseInTabBar(m.mouseY) {
			var cmd tea.Cmd
			m, cmd = m.handleTabBarClick()
			return m, cmd
		}
		return m, nil

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
		if m.contextMenu.active {
			return m.handleContextMenuKey(msg)
		}

		// Paste: Ctrl+V, Ctrl+Shift+V, or Shift+Insert.
		// Linux terminals often lack xclip; native X11/Wayland
		// clipboard access is tried first, then OSC52.
		if isPasteKey(msg) && m.pasteTarget() != pasteTargetNone {
			return m.handlePasteKey()
		}

		// F1-F4 tab switching: handle globally so function keys are not
		// swallowed by text inputs (common on Linux when the terminal
		// does not remap F-keys before they reach the TUI).
		if n := functionKeyNum(msg); n > 0 {
			if m, cmd, ok := m.handleFunctionKey(n); ok {
				return m, cmd
			}
		}

		// --- Codex "waiting for auth" overlay ---
		// Esc dismisses the overlay; the OAuth flow keeps
		// running in the background and will surface its
		// result through the existing codexLoginSuccessMsg
		// / codexLoginFailedMsg handlers.
		if m.codexAuthPending && (msg.String() == "esc" || msg.String() == "Esc") {
			m.codexAuthPending = false
			return m, nil
		}

		// --- "Updating cortex" overlay ---
		// Esc dismisses the overlay in the error phase.
		// Enter on the green "All done!" screen restarts
		// with the freshly installed binary.
		if m.updateOverlay.active {
			switch m.updateOverlay.phase {
			case "error":
				if msg.String() == "esc" || msg.String() == "Esc" ||
					msg.String() == "enter" || msg.String() == "Enter" {
					m.updateOverlay.active = false
					return m, nil
				}
				return m, nil
			case "done", "restarting":
				if msg.String() == "enter" || msg.String() == "Enter" {
					return m, m.execSelfCmd()
				}
				return m, nil
			default:
				return m, nil
			}
		}
		// --- Global quit confirm overlay ---
		if msg.String() == "ctrl+c" {
			if m.activeTab == TabKindChat {
				if cmd := m.copyChatSelectionCmd(); cmd != nil {
					return m, cmd
				}
			}
		}

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
				// goroutine. The user will see an immediate
				// "Opening ChatGPT sign-in in your browser…"
				// status line (so the action feels instant even
				// before the browser appears), then their browser
				// opens, they sign in, and we fire a tea.Cmd when
				// done so the model can switch back to the chat
				// without blocking the UI.
				//
				// The payload is the spec to switch to after the
				// OAuth flow succeeds. The right-panel model
				// picker passes the chosen model spec; the
				// right-panel key manager passes "codex:" with no
				// pending model (we don't switch the active model
				// just because the user re-authenticated — they
				// can still pick a codex model afterwards via
				// /model).
				pendingModel := payload
				sess.rightPanel.Close()
				m.updateChatWidth()
				sess.input.Focus()
				sess.focus = FocusEditor
				// If the payload is "codex:" (key manager path)
				// we still want to start the OAuth flow but
				// don't auto-switch the active model afterwards.
				// startCodexLoginCmd handles "" as "no pending
				// model" already.
				if pendingModel == "codex:" {
					pendingModel = ""
				}
				return m, tea.Batch(
					m.emitStatusMsg("Opening ChatGPT sign-in in your browser…", StatusMsgInfo),
					m.startCodexLoginCmd(pendingModel),
				)
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
			if m.selectedSession < len(m.sessions)-1 {
				m.selectedSession++
				m.activeTab = TabKindChat
				selSess := m.sessions[m.selectedSession]
				selSess.unreadCount = 0
				selSess.input.SetWidth(m.width - 4)
				if selSess.client == nil && !selSess.reconnecting {
					selSess.reconnecting = true
					cmds = append(cmds, m.reconnectSession(selSess, false))
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
			if m.selectedSession > 0 {
				m.selectedSession--
				m.activeTab = TabKindChat
				selSess := m.sessions[m.selectedSession]
				selSess.unreadCount = 0
				selSess.input.SetWidth(m.width - 4)
				if selSess.client == nil && !selSess.reconnecting {
					selSess.reconnecting = true
					cmds = append(cmds, m.reconnectSession(selSess, false))
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
			cmds = append(cmds, m.emitStatusMsg("New session created", StatusMsgInfo))
			// Continue the switch logic to wire up the
			// session properly.
			m.activeTab = TabKindChat
			m.persistSessions()
			cmds = append(cmds, m.reconnectSession(newSess, false))
			return m, tea.Batch(cmds...)

		case "ctrl+b":
			// Toggle the right-side info / status panel
			// (OpenCode-style). The panel is read-only and
			// shows: active model, context window usage,
			// elapsed time, queued message count, and a
			// compact keybind legend. The chat input keeps
			// focus so the user can keep typing while
			// glancing at the stats.
			if sess != nil {
				sess.rightPanel.Toggle()
				m.updateChatWidth()
			}
			return m, nil

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
						cmds = append(cmds, m.reconnectSession(selSess, false))
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
				cmds = append(cmds, m.reconnectSession(newSess, false))
				return m, tea.Batch(cmds...)
			case "x":
				if idx, ok := m.sessionsSelectedIdx(); ok {
					m.sessionCloseIdx = idx
					m.sessionCloseSelected = 1 // default No
					m.state = StateSessionCloseConfirm
				}
				return m, nil
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
						providerName := m.settingsKeys[m.settingsKeySel].Provider
						// OAuth providers don't have an API key or a
						// base URL the user should edit — they sign in
						// with their existing subscription. Skip the
						// wizard entirely and fire the OAuth flow
						// directly. The user sees a status line and
						// their browser opens to auth.openai.com (or
						// claude.ai / github.com for the other OAuth
						// providers) immediately.
						if cortexconfig.ProviderAuthKind(providerName) == "oauth" {
							cmds = append(cmds, m.emitStatusMsg("Opening "+cortexconfig.ProviderDisplayName(providerName)+" sign-in in your browser…", StatusMsgInfo))
							cmds = append(cmds, m.startOAuthLoginCmd(providerName))
							return m, tea.Batch(cmds...)
						}
						// Local servers (Ollama, LM Studio, vLLM) and
						// env-var providers (Bedrock) just need a
						// base URL — the wizard hides the API-key
						// field for them. Built-in API-key providers
						// (OpenAI, Anthropic, etc.) show all three
						// fields as before.
						m.openSettingsWizard(providerName)
					}
				case "a":
					m.openSettingsTextInput(settingsInputCustomProviderName, "", "New provider name", "e.g. groq, together, local-ai", "", false)
				case "r":
					if m.settingsKeySel < len(m.settingsKeys) {
						providerName := m.settingsKeys[m.settingsKeySel].Provider
						cmds = append(cmds, m.emitStatusMsg("Refreshing models for "+providerName, StatusMsgInfo), m.fetchModelsForProvider(providerName))
					}
				case "tab":
					// Move to Other Settings. The 1 here is the
					// section index (Providers=0, Other Settings=1)
					// and matches the `sectionIdx` helper in
					// tabs.go.
					m.settingsActiveSection = 1
				case "shift+tab":
					m.settingsActiveSection = 1
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
					case 5: // Auto-compact context — toggle
						m.setConfiguredAutoCompact(!m.configuredAutoCompact())
						state := "on"
						if !m.configuredAutoCompact() {
							state = "off"
						}
						cmds = append(cmds, m.emitStatusMsg("Auto-compact context: "+state+" (use /compact to run manually)", StatusMsgInfo))
					}
				case "tab":
					// Move back to Providers (section 0).
					m.settingsActiveSection = 0
				case "shift+tab":
					m.settingsActiveSection = 0
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
				sess.focus = FocusEditor
				sess.input.Focus()
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

		// Login picker (opened by /login slash command)
		if m.loginPicker.IsVisible() {
			switch msg.String() {
			case "up", "k":
				m.loginPicker.MoveUp()
			case "down", "j":
				m.loginPicker.MoveDown()
			case "esc":
				m.loginPicker.Close()
			case "enter":
				provider, wantDevice := m.loginPicker.Selected()
				m.loginPicker.Close()
				if provider == "codex" && wantDevice {
					cmds = append(cmds,
						m.emitStatusMsg("Requesting ChatGPT (codex) one-time code (15-min window)…", StatusMsgInfo),
						m.startCodexDeviceLoginCmd(""),
					)
				} else if provider == "codex" {
					return m, tea.Batch(
						m.emitStatusMsg("Opening ChatGPT (codex) sign-in in your browser…", StatusMsgInfo),
						m.startCodexLoginCmd(""),
					)
				} else {
					// claude-sub / copilot: env-var hint
					cmds = append(cmds, m.startOAuthLoginCmd(provider))
				}
			case "backspace":
				q := m.loginPicker.Query()
				if len(q) > 0 {
					m.loginPicker.SetQuery(q[:len(q)-1])
				}
			default:
				if isPickerFilterKey(msg.String()) {
					m.loginPicker.SetQuery(m.loginPicker.Query() + msg.String())
				}
			}
			return m, tea.Batch(cmds...)
		}

		// Model picker (opened by /model slash command)
		if m.modelPicker.IsVisible() {
			switch msg.String() {
			case "up", "k":
				m.modelPicker.MoveUp()
			case "down", "j":
				m.modelPicker.MoveDown()
			case "esc":
				m.modelPicker.Close()
			case "enter":
				spec := m.modelPicker.Selected()
				m.modelPicker.Close()
				if spec != "" {
					cmds = append(cmds, m.applyModelPickerSelection(spec))
				}
			case "backspace":
				q := m.modelPicker.Query()
				if len(q) > 0 {
					m.modelPicker.SetQuery(q[:len(q)-1], m.cortexCfg)
				}
			default:
				// Treat printable key strings as filter input.
				// We re-use the key string instead of msg.Rune()
				// (which doesn't exist on tea.KeyPressMsg in the
				// current bubbletea v2) and we filter out all
				// control / navigation keys by length and the
				// bubbletea naming convention.
				ks := msg.String()
				if isPickerFilterKey(ks) {
					m.modelPicker.SetQuery(m.modelPicker.Query()+ks, m.cortexCfg)
				}
			}
			return m, tea.Batch(cmds...)
		}

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
				// Capture the input text BEFORE clearing
				// it; some slash commands need the arg portion.
				rawText := strings.TrimSpace(sess.input.Value())
				sess.input.SetValue("")
				sess.input.SetHeight(1)
				if action != "" {
					cmds = append(cmds, m.handleCommandAction(action, sess, rawText)...)
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
		// after the in-flight edit finishes.
		// (The previous focus-cycling behavior via Tab to "enter
		// chat scroll mode" has been removed; chat scrolling now
		// works on the Chat tab regardless of focus state or
		// sub-panels. Use mouse wheel or pgup/pgdn/arrows/k/j
		// to scroll the chat at any time.)
		if msg.String() == "tab" {
			if sess.focus == FocusEditor && (sess.agentState == StateStreaming ||
				sess.agentState == StateToolExecuting || sess.agentState == StatePlanExecuting) &&
				strings.TrimSpace(sess.input.Value()) != "" {
				return m.submitFromInput(sess, true)
			}
			// No more focus cycling on Tab. If the user presses Tab
			// with no text while idle, we simply ignore it here
			// (textarea may still see it). Scroll the chat directly.
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

		case "shift+tab":
			// (workflow mode cycling removed)
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
				// Reset the state to WaitingForInput so the
				// user can submit a follow-up message. The
				// previous version of this code only stopped
				// the spinner and sent the cancel, leaving
				// agentState == StateStreaming — which meant
				// the submit path was a no-op on the next
				// Enter press and the user reported "send a
				// new one, nothing happens". The goroutine
				// running runTurn will eventually observe
				// ctx.Err() and emit agent_done, which is a
				// no-op against the already-WaitingForInput
				// state.
				sess.agentState = StateWaitingForInput
				sess.input.Focus()
				cmds = append(cmds, m.maybeAutoCompact())
				return m, tea.Batch(cmds...)
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

		// Chat scrolling keys: work whenever the Chat tab is active,
		// no matter what the current focus (input, right panel, etc) or
		// other panels. This removes the need for the (now removed)
		// Tab key "focus command" to switch into a scrollable chat mode.
		// Mouse wheel (see MouseWheelMsg) provides an additional way
		// to scroll chat from anywhere.
		if m.activeTab == TabKindChat && sess != nil {
			// Preserve ↑ history recall: if we're at the top of the
			// input in waiting state, let the later check open the
			// history panel instead of scrolling.
			if msg.String() == "up" && sess.agentState == StateWaitingForInput &&
				sess.input.Line() == 0 && sess.input.Column() == 0 && len(sess.history.entries) > 0 {
				// do not scroll; fall through to history open
			} else {
				didScroll := false
				switch msg.String() {
				case "up":
					sess.chatScrollOffset += 3
					didScroll = true
				case "down":
					sess.chatScrollOffset -= 3
					didScroll = true
				case "pgup":
					sess.chatScrollOffset += 20
					didScroll = true
				case "pgdown":
					sess.chatScrollOffset -= 20
					didScroll = true
				case "home":
					sess.chatScrollOffset = m.sessionMaxScrollOffset(sess)
					didScroll = true
				case "end", "G":
					sess.chatScrollOffset = 0
					didScroll = true
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
				if didScroll {
					m.clampScrollOffset(sess)
					return m, nil
				}
			}
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
			cmds = append(cmds, m.reconnectSession(sess, m.forceInit))
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
			_ = sess.client.SendRestoreHistory(chatMessagesToProviderHistory(sess.chatMessages))
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
			return m, m.reconnectSession(sess, m.forceInit)
		}
		return m, nil

	case tea.ClipboardMsg:
		if msg.Content != "" && m.pasteTarget() != pasteTargetNone {
			return m.applyPasteText(msg.Content)
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

	case SignalQuitMsg:
		sess := m.currentSession()
		if sess != nil && sess.client != nil {
			sess.client.SendCancel()
			sess.client.SendClose()
		}
		m.persistSessions()
		return m, tea.Quit

	case clearStatusMsgMsg:
		if msg.gen == m.statusMsg.gen {
			// Reset to empty (no spinner) so subsequent
			// messages don't accidentally show a
			// frozen spinner frame.
			m.statusMsg = StatusMessage{Spinner: -1}
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

	case codexLoginStartedMsg:
		// The codex OAuth flow has just been kicked off.
		// Show the full-screen "waiting for auth" overlay
		// with the URL so the user can copy it into a
		// browser manually if the auto-open fails (headless
		// / WSL / SSH).
		m.codexAuthPending = true
		m.codexAuthURL = msg.authorizeURL
		m.codexAuthModel = msg.pendingModel
		m.codexAuthStartedAt = time.Now()
		// Also surface a quick status-bar line so the
		// user knows the flow has started even before
		// they look at the overlay.
		return m, m.emitStatusMsg("Waiting for ChatGPT sign-in. If your browser didn't open, copy the URL from the overlay.", StatusMsgInfo)

	case codexLoginSuccessMsg:
		// Clear the "waiting for auth" overlay.
		m.codexAuthPending = false
		return m, m.handleCodexLoginSuccess(msg.pendingModel, msg.email, msg.planType)

	case codexLoginFailedMsg:
		m.codexAuthPending = false
		return m, m.handleCodexLoginFailed(msg.err, msg.authorizeURL)

	case compactMsg:
		// /compact (or auto-compact) finished. Show the
		// before/after stats in the status bar. We also
		// clear compactInFlight HERE (not inside the
		// tea.Cmd closure) so the spinner stops
		// animating only when the result is actually
		// rendered. The previous version cleared the
		// flag inside compactCmd, which meant the
		// spinner stopped before the result message
		// landed and the user saw "compacting…"
		// freeze. CodeRabbit flagged this in PR #2.
		m.compactInFlight = false
		m.statusMsg.Spinner = -1
		return m, m.handleCompactMsg(msg)

	case selfUpdateFinishedMsg:
		return m.handleSelfUpdateFinished(msg)

	case selfUpdateProgressMsg:
		return m.handleSelfUpdateProgress()

	case compactProgressMsg:
		// Advance the spinner frame while compaction is
		// in-flight. The compactInFlight flag is set when
		// /compact starts and cleared when compactMsg fires.
		if m.compactInFlight {
			m.statusMsg.Spinner = (m.statusMsg.Spinner + 1) % 8
			m.statusMsg.gen++ // bump gen so auto-clear doesn't wipe mid-compaction
			return m, compactProgressTick()
		}
		// Compaction done — don't reschedule the tick.
		return m, nil

	case autosaveTickMsg:
		// Periodic crash-safety persistence. This is
		// intentionally lightweight and uses the same
		// persistSessions path as quit/SIGINT. Empty
		// sessions are filtered there, so autosave won't
		// clutter the Sessions tab.
		if !m.testMode {
			m.persistSessions()
			return m, autosaveTick()
		}
		return m, nil

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

	case updateOverlayStartRestartMsg, updateOverlayTickMsg, updateOverlayExecMsg:
		return m.handleUpdateOverlayExec()

	case updateOverlayDismissMsg:
		return m.handleUpdateOverlayDismiss(), nil
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
		// Start the per-turn timer so the "⏱ 0:42" indicator
		// counts only while the agent is working, not wall
		// clock since session open.
		sess.StartTurn()
		animCmd := sess.thinkingAnim.Start()

		if sess.client != nil {
			sess.client.SendInput(displayText, attachments)
		} else {
			// No client yet — the reconnect goroutine is
			// still in flight. Surface a status message
			// instead of silently dropping the user's
			// message AND spinning the thinking anim.
			// The previous version of this code did both:
			// it set StateWaitingForInput but also called
			// animCmd.Start(), so the spinner ran forever
			// even though nothing was actually being
			// processed. The user reported "Ctrl+T new
			// session, send prompt, loads forever".
			//
			// Fix: restore the text, leave the state at
			// WaitingForInput, do NOT start the spinner,
			// and emit a clear status message. The user
			// presses Enter again once the reconnect
			// finishes (the session.client is set by the
			// reconnectSuccessMsg handler).
			sess.input.SetValue(displayText)
			sess.agentState = StateWaitingForInput
			sess.thinkingAnim.Stop()
			return m, m.emitStatusMsg("Reconnecting to daemon… press Enter again in a moment", StatusMsgWarning)
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

// View implements tea.Model — builds all content fresh each frame.
func (m Model) View() tea.View {
	if m.width == 0 {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		v.MouseMode = m.viewMouseMode()
		return v
	}

	sess := m.currentSession()

	// If a codex OAuth flow is in flight, show the big
	// "waiting for auth" overlay on top of whatever the
	// user is doing. They can dismiss it with Esc to
	// continue using the TUI while the browser round-trip
	// continues in the background.
	if m.codexAuthPending {
		v := tea.NewView(m.renderCodexAuthOverlay())
		v.AltScreen = true
		v.MouseMode = m.viewMouseMode()
		return v
	}

	// Big "Updating cortex" overlay. The user wanted a
	// big popup with a cool animation that takes over the
	// TUI while the self-update runs. We render it on
	// top of the main view so the user can still see
	// the chat behind it (semi-transparent feel from
	// the centered modal). When the update finishes
	// successfully we re-exec the binary so the user
	// comes back to a fresh TUI.
	if m.updateOverlay.active {
		v := tea.NewView(m.renderUpdateOverlay())
		v.AltScreen = true
		v.MouseMode = m.viewMouseMode()
		return v
	}

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
	// When the Chat tab is active the chat viewport is the primary
	// scrollable area (scrolling works regardless of FocusChat etc).
	viewportFocused := m.activeTab == TabKindSessions || m.activeTab == TabKindSettings || m.activeTab == TabKindChat
	tabBarWidth := layout.ChatWidth
	if m.activeTab == TabKindSessions || m.activeTab == TabKindSettings {
		tabBarWidth = m.width
	}
	hoverTab := -1
	if kind, ok := m.hoverTabKind(); ok {
		hoverTab = int(kind)
	}
	tabBar := renderTabBar(m.activeTab, tabBarWidth, m.styles, viewportFocused, m.tabAlertBlinkOn, hoverTab)
	uv.NewStyledString(tabBar).Draw(canvas, image.Rect(0, y, tabBarWidth, y+layout.TabBarHeight))
	y += layout.TabBarHeight

	switch m.activeTab {
	case TabKindSessions:
		sessionsHeight := m.height - layout.TabBarHeight - layout.StatusBarHeight
		sv := renderSessionsView(m.sessions, m.width, sessionsHeight, m.styles, m.sessionsInput.Value(), m.sessionsInput.View(), m.sessionsSelected)
		uv.NewStyledString(sv).Draw(canvas, image.Rect(0, y, m.width, y+sessionsHeight))
		y += sessionsHeight

	case TabKindChat:
		chatLines := m.visibleChatLines(sess, layout)

		// Always use the focused border style for the main content area
		// (chat box) so the outline color is consistent across all tabs
		// (F1 Sessions, F2 Chat, F3 Settings). Other tabs' renderers
		// (renderSessionsView, renderSettingsView) also use FocusedStyle.
		// The right panel uses its own (blurred) style for visual separation.
		if sess != nil && sess.chatSel.active {
			top, _, left, _ := m.chatInnerBounds()
			selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#2A4A7F")).Foreground(lipgloss.Color("#FFFFFF"))
			chatLines = applyChatSelectionHighlight(chatLines, top, left, sess.chatSel, selStyle)
		}

		var chatBorderStyle = m.styles.ViewportFocusedStyle
		chatBox := chatBorderStyle.Width(layout.ChatWidth).Height(layout.ChatHeight).
			Render(strings.Join(chatLines, "\n"))
		uv.NewStyledString(chatBox).Draw(canvas, image.Rect(0, y, layout.ChatWidth, y+layout.ChatHeight))

		// Right panel
		if sess != nil && sess.rightPanel.IsVisible() {
			rpHeight := layout.ChatHeight + 1
			infoView := m.buildRightPanelInfoView(sess)
			rpView := sess.rightPanel.View(rpHeight, m.styles, sess.focus == FocusRightPanel, sess.modelName, sess.todos, infoView)
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
			inputSection = renderInputBox("Chat", false, "", m.width, false, m.styles.ColorBlurBorder)
		} else if sess != nil {
			inputSection = renderInputBox("Chat", false, sess.input.View(), m.width, sess.focus == FocusEditor, m.styles.ColorBlurBorder)
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
			AutoCompact:     m.configuredAutoCompact(),
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
	statusBar := renderStatusBar(m.width, connected, reconnecting, m.statusMsg, m.styles, m.buildStatusBarInfo(m.currentSession()))
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

	// Login picker overlay: drawn on top of everything else
	// (above slash menu, above model picker, above right panel).
	if m.loginPicker.IsVisible() {
		pickerH := m.loginPicker.VisibleHeight()
		if pickerH > m.height-4 {
			pickerH = m.height - 4
		}
		if pickerH < 6 {
			pickerH = 6
		}
		overlay := m.loginPicker.View(m.width, pickerH, m.styles)
		h := lipgloss.Height(overlay)
		popupY := (m.height - h) / 2
		if popupY < 0 {
			popupY = 0
		}
		uv.NewStyledString(overlay).Draw(canvas, image.Rect(0, popupY, m.width, popupY+h))
	}

	// Model picker overlay: drawn on top of everything else
	// (above slash menu, above right panel). Same idea as
	// quit-confirm: full-screen modal so the user can't miss it.
	if m.modelPicker.IsVisible() {
		pickerH := m.modelPicker.VisibleHeight()
		if pickerH > m.height-4 {
			pickerH = m.height - 4
		}
		if pickerH < 6 {
			pickerH = 6
		}
		overlay := m.modelPicker.View(m.width, pickerH, m.styles)
		h := lipgloss.Height(overlay)
		popupY := (m.height - h) / 2
		if popupY < 0 {
			popupY = 0
		}
		uv.NewStyledString(overlay).Draw(canvas, image.Rect(0, popupY, m.width, popupY+h))
	}

	m.drawContextMenu(canvas)

	content := strings.ReplaceAll(canvas.Render(), "\r\n", "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = m.viewMouseMode()
	return v
}

// --- Helper methods ---


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
	sess.assistantLastRenderAt = time.Time{}
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
	if m.isWelcomeScreen(sess) {
		lines := buildWelcomeLines(m.mdRenderer.width, m.styles)
		maxOff := len(lines) - contentHeight
		if maxOff < 0 {
			return 0
		}
		return maxOff
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

	return *m, m.connectForkSession(newSess, forkSessionID, sep.TurnIdx, activeModel)
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
		reconnectCmd = m.reconnectSession(newSess, false)
	}

	// Clamp the sessions tab cursor to a valid row after
	// a close. The user reported: "the selector arrow
	// starts at the bottom and not at the top". After a
	// session is closed, the previous "n-1" clamp left
	// the cursor at the bottom of the visible list. The
	// fix: keep the cursor where it was if still in range,
	// otherwise move it to the top (index 0 = newest, per
	// the new sort order).
	if n := m.sessionsVisibleCount(); n > 0 {
		if m.sessionsSelected >= n {
			m.sessionsSelected = 0
		}
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

// visibleSessionIndices returns the indices of sessions that match the
// current filter, sorted by creation time (newest first) to match the
// order used by renderSessionsView. The user reported: "sort sessions
// by date from newest (top) to oldest (bottom)" and "for the session
// selection the selector arrow starts at the bottom and not at the top
// it should be at the highest session". The selector arrow follows
// sessionsSelected, which is an index into the visible list. If the
// visible list was in creation order (oldest first), sessionsSelected=0
// would land on the oldest session at the bottom of the new sorted
// display. Sorting the visible indices here too means
// sessionsSelected=0 always points to the newest session at the top.
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
	// Sort by createdAt descending so index 0 = newest.
	// (See docstring above.)
	sort.SliceStable(indices, func(i, j int) bool {
		return m.sessions[indices[i]].createdAt.After(m.sessions[indices[j]].createdAt)
	})
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

// emitStatusMsg sets the global transient status bar message and returns a
// tea.Cmd that clears it after 3 seconds. Rapid successive calls are safe
// because each call bumps the generation counter; only the matching clear fires.
func (m *Model) emitStatusMsg(text string, kind StatusMsgKind) tea.Cmd {
	m.statusMsg.gen++
	m.statusMsg.Text = text
	m.statusMsg.Kind = kind
	m.statusMsg.Spinner = -1 // no spinner by default; /update sets its own
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
	return "Ask the agent anything... (Enter to send, Shift+Enter or Alt+Enter for new line)"
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
