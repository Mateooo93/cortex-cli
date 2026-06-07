package ui

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"charm.land/bubbles/v2/textarea"
	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/workflow"
)

// SessionState holds all accumulated UI state for a single agent session.
// Sessions are independent objects — the Chat tab renders whichever session
// is currently selected. Messages accumulate continuously from daemon events
// regardless of which tab is visible.
type SessionState struct {
	// daemonSessionID is the session ID assigned by the daemon after the
	// initial handshake. It is used as the stable key carried by all async
	// goroutines (event loops, reconnect attempts) so the Update handler can
	// locate the right session even after the sessions slice has been
	// re-ordered by a close operation. It changes on every successful
	// reconnect, which naturally invalidates any in-flight messages from the
	// previous connection without needing a separate generation counter.
	// Empty for sessions that have never successfully connected.
	daemonSessionID string

	// Daemon connection
	client       *daemon.SessionClient
	reconnecting bool
	initState    protocol.InitState

	// Accumulated chat display — built from daemon events
	chatMessages     []ChatMessage
	chatScrollOffset int

	// Live streaming buffers
	assistantBuf      string
	assistantRendered string
	thinkingBuf       string
	thinkingRendered  string
	showThinking      bool

	// Agent / workflow state
	agentState     AppState
	activeWorkflow string
	workflows      []protocol.WorkflowInfo
	activePlan     *protocol.Plan
	todos          []protocol.TodoItem

	// Token accounting
	inputTokens                  int64
	outputTokens                 int64
	cacheCreationTokens          int64
	cacheReadTokens              int64
	lastOutputTokens             int64
	turnStartInputTokens         int64
	turnStartOutputTokens        int64
	turnStartCacheCreationTokens int64
	turnStartCacheReadTokens     int64
	elapsed                      time.Duration

	// Confirm / question state
	confirmToolName    string
	confirmDetailShown bool

	// Pending messages
	pendingInput      *pendingMsg
	pendingPlanAction *pendingPlanAction
	pendingTools      map[string]int

	// Panels
	// rightPanel is visible by default in info mode so the
	// user can see model / context / keybinds from the first
	// paint. Ctrl+B toggles it.
	rightPanel         RightPanel
	workflowGraphPanel WorkflowGraphPanel
	questionPanel      QuestionPanel
	attachmentPanel    AttachmentPanel
	historyPanel       HistoryPanel

	// Input area
	input         textarea.Model
	focus         FocusState
	fileCompleter FileCompleter
	slashMenu     SlashMenu

	// Animation
	thinkingAnim ThinkingAnim

	// Input recall history (.cortex/history.txt)
	history *History

	// Current model name
	modelName string

	// unreadCount is the number of completed agent responses that arrived
	// while this session was not the active workspace view.
	unreadCount int

	// Trim confirm state
	trimPrevState AppState
	trimSelected  int
	trimSep       TurnSepInfo

	// Fork lineage (zero values for root sessions)
	parentID    string
	forkTurnIdx int

	// Persist metadata
	//
	// label is a user-friendly name shown in the Sessions tab when
	// chatMessages is empty. It is persisted across restarts.
	label string
	// createdAt is the time this session was first opened. Used for
	// sorting + display in the Sessions tab.
	createdAt time.Time

	// workflowEngine is per-session so workflows started in
	// one session don't bleed into another. When the user
	// switches tabs, this engine is replaced.
	workflowEngine *workflow.Engine
	// turnElapsed accumulates the time the agent has actually
	// spent thinking/streaming for the current turn. We use this
	// for the "⏱  2:13" indicator in the slim footer + right
	// panel — the user asked the timer to count only when the
	// agent is working, not the wall-clock since session open.
	turnElapsed   time.Duration
	turnActive    bool
	turnStartedAt time.Time
	// persistID is a stable identifier for the saved-sessions file.
	// For live sessions it equals daemonSessionID; for restored
	// placeholders it's the ID we wrote to disk.
	persistID string
}

// newSessionState initialises a fresh session state ready for a new agent session.
func newSessionState(cfg *config.Config, client *daemon.SessionClient) *SessionState {
	s := &SessionState{
		agentState:     StateWaitingForInput,
		input:          newInput(),
		thinkingAnim:   NewThinkingAnim(),
		questionPanel:  NewQuestionPanel(),
		focus:          FocusEditor,
		client:         client,
		modelName:      cfg.Model,
		history:        NewHistory(cfg.Paths.Primary()),
		showThinking:   config.ShowThinking(),
		createdAt:      time.Now(),
		rightPanel:     NewRightPanel(),
	}
	if client != nil {
		s.daemonSessionID = client.SessionID()
		s.persistID = s.daemonSessionID
	} else {
		// Allocate a unique placeholder daemonSessionID for
		// sessions that haven't been connected yet. The
		// reconnect-success handler matches the new client
		// back to the right session via
		// findSessionByDaemonID, which compares against
		// SessionState.daemonSessionID. If we left this
		// empty, multiple "not yet connected" sessions
		// (one freshly created by Ctrl+T, plus any
		// restored placeholders from disk) would all share
		// the empty string, and findSessionByDaemonID("")
		// would return the first one in the slice — not
		// necessarily the session that just dispatched
		// the reconnect. The user's bug report was "I make
		// a new session, the AI doesn't respond at all":
		// the new client's events were being routed to a
		// stale restored session, while the freshly-
		// created Ctrl+T session stayed with client=nil
		// forever, so any submit went through the
		// "Reconnecting to daemon…" branch. Fix: each
		// not-yet-connected session gets a unique random
		// placeholder, which is replaced with the real
		// daemonSessionID once the reconnect completes.
		s.daemonSessionID = "pending-" + randomHex(8)
	}
	return s
}

// randomHex returns a random hex string of the given byte
// length. Used only for the placeholder daemonSessionID on
// not-yet-connected sessions — never for security-sensitive
// values.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely (rand.Read only fails if the
		// OS crypto source is broken); fall back to a
		// timestamp so we still have a unique value.
		return time.Now().Format("20060102150405.000000")
	}
	return hex.EncodeToString(b)
}

// TurnElapsed returns the accumulated time the agent has spent
// working on the current turn. While the agent is busy
// (agentState == StateStreaming / StateToolExecuting) the value
// keeps ticking up; while it's idle it returns the accumulated
// total. The user asked for a "turn timer" instead of a
// wall-clock-since-session-open indicator.
func (s *SessionState) TurnElapsed() time.Duration {
	if s.turnActive {
		return s.turnElapsed + time.Since(s.turnStartedAt)
	}
	return s.turnElapsed
}

// StartTurn begins accumulating elapsed time. Call this when the
// agent begins processing a new user turn.
func (s *SessionState) StartTurn() {
	if s.turnActive {
		return
	}
	s.turnActive = true
	s.turnStartedAt = time.Now()
}

// FinishTurn stops accumulating and returns the final elapsed
// for the turn. The next call to StartTurn() resets the clock
// for the next user turn.
func (s *SessionState) FinishTurn() time.Duration {
	if s.turnActive {
		s.turnElapsed += time.Since(s.turnStartedAt)
		s.turnActive = false
	}
	d := s.turnElapsed
	s.turnElapsed = 0
	return d
}

// EnsureWorkflowEngine returns a per-session workflow engine.
// First call creates the engine bound to the user's config; later
// calls return the same instance. We make the engine per-session
// because each session is a separate conversation context and the
// user expects workflow state to reset when they start a new chat.
func (s *SessionState) EnsureWorkflowEngine(cfg *cortexconfig.Config) *workflow.Engine {
	if s.workflowEngine == nil {
		s.workflowEngine = workflow.NewEngine(cfg)
	}
	return s.workflowEngine
}
