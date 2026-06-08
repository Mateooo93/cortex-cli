package ui

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"charm.land/bubbles/v2/textarea"
	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

// RecentToolStatus is the lifecycle state of a
// single tool call as it appears in the bottom
// activity strip. The strip is rendered as a FIFO
// of these entries.
type RecentToolStatus int

const (
	RecentToolPending RecentToolStatus = iota // tool call issued, no result yet
	RecentToolDone                            // tool returned successfully
	RecentToolFailed                          // tool returned an error
)

// RecentToolEntry is one row in the bottom activity
// strip. The user requested a Claude-Code-style
// compact view of "sub-agents" (i.e. tool calls the
// main agent is making) at the bottom of the chat.
// We track each tool call with enough metadata to
// render a one-liner like:
//
//	● read_file  internal/ui/model.go    2.1s
//	✓ edit_file  internal/ui/foo.go      done
//	✗ run_shell  npm test               failed
//
// The StartedAt time drives the elapsed timer; the
// strip auto-fades old entries off the end of the
// buffer once we hit recentToolsMax (5).
type RecentToolEntry struct {
	ToolID    string
	Name      string
	Summary   string
	StartedAt time.Time
	EndedAt   time.Time
	Status    RecentToolStatus
	// IsError is true when the tool returned a
	// non-zero exit / error. Drives the "✗"
	// prefix in the strip.
	IsError bool
}

const recentToolsMax = 5

// pushRecentTool appends a new entry to the
// session's recent-tools FIFO, dropping the oldest
// entry if the buffer is full. Pure function over
// the entry slice — no locking required since the
// caller (the Update goroutine) is the only writer.
func (s *SessionState) pushRecentTool(e RecentToolEntry) {
	if len(s.RecentTools) >= recentToolsMax {
		// Drop the oldest. The slice is small (max
		// 5) so a simple in-place shift is fine.
		s.RecentTools = s.RecentTools[1:]
	}
	s.RecentTools = append(s.RecentTools, e)
}

// markRecentToolDone updates the status (and
// EndedAt time) of the most recent pending entry
// matching toolID. No-op if no match is found
// (e.g. the tool result arrived for a tool we
// didn't see the start of, which can happen after
// a session restore).
func (s *SessionState) markRecentToolDone(toolID string, isError bool) {
	for i := len(s.RecentTools) - 1; i >= 0; i-- {
		if s.RecentTools[i].ToolID == toolID && s.RecentTools[i].Status == RecentToolPending {
			s.RecentTools[i].EndedAt = time.Now()
			s.RecentTools[i].IsError = isError
			if isError {
				s.RecentTools[i].Status = RecentToolFailed
			} else {
				s.RecentTools[i].Status = RecentToolDone
			}
			return
		}
	}
}

// hasPendingRecentTools reports whether any tool in the activity strip
// is still running.
func (s *SessionState) hasPendingRecentTools() bool {
	for _, e := range s.RecentTools {
		if e.Status == RecentToolPending {
			return true
		}
	}
	return false
}

// activityStripRows returns how many terminal rows the bottom activity
// strip should occupy (0 or 1).
func runningBackgroundProcesses(procs []protocol.BackgroundProcessItem) []protocol.BackgroundProcessItem {
	var out []protocol.BackgroundProcessItem
	for _, p := range procs {
		if p.Running {
			out = append(out, p)
		}
	}
	return out
}

func (s *SessionState) hasRunningBackgroundProcesses() bool {
	return len(runningBackgroundProcesses(s.backgroundProcesses)) > 0
}

func (s *SessionState) activityStripRows() int {
	if s == nil || len(s.RecentTools) == 0 {
		return 0
	}
	return 1
}

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
	chatSel          chatSelection

	// Live streaming buffers
	assistantBuf      string
	assistantRendered string
	streamCache       streamDisplayCache
	streamPending     string
	streamPlayback    StreamPlayback
	// Per-turn stats captured from the latest stream_done for the footer line.
	lastTurnInputTokens  int64
	lastTurnOutputTokens int64
	lastTurnCacheCreate  int64
	lastTurnCacheRead    int64
	thinkingBuf           string
	thinkingRendered      string
	showThinking          bool

	// backgroundProcesses lists shell commands still running (or
	// recently exited) that the agent started via run_shell.
	backgroundProcesses []protocol.BackgroundProcessItem

	// RecentTools is a compact FIFO of the last few
	// tool calls made by the main agent. The UI
	// renders this as a Claude-Code-style activity
	// strip at the bottom of the chat tab so the
	// user can see at a glance what the agent is
	// currently doing (e.g. "● read_file
	// internal/ui/model.go" with a spinner) and
	// what's about to be sent to the model. We
	// keep at most 5 entries (the strip is 1 line
	// tall; more would overflow).
	RecentTools []RecentToolEntry

	// Agent state
	agentState AppState
	activePlan *protocol.Plan
	todos      []protocol.TodoItem

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
	rightPanel      RightPanel
	questionPanel   QuestionPanel
	attachmentPanel AttachmentPanel
	historyPanel    HistoryPanel

	// Input area
	input         textarea.Model
	focus         FocusState
	fileCompleter FileCompleter
	slashMenu     SlashMenu

	// Animation
	thinkingAnim     ThinkingAnim
	toolActivityAnim ToolActivityAnim

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
		agentState:    StateWaitingForInput,
		input:         newInput(),
		thinkingAnim:       NewThinkingAnim(),
		toolActivityAnim:   NewToolActivityAnim(),
		streamPlayback: NewStreamPlayback(),
		questionPanel: NewQuestionPanel(),
		focus:         FocusEditor,
		client:        client,
		modelName:     cfg.Model,
		history:       NewHistory(cfg.Paths.Primary()),
		showThinking:  config.ShowThinking(),
		createdAt:     time.Now(),
		rightPanel:    NewRightPanel(),
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
	s.turnStartInputTokens = s.inputTokens
	s.turnStartOutputTokens = s.outputTokens
	s.turnStartCacheCreationTokens = s.cacheCreationTokens
	s.turnStartCacheReadTokens = s.cacheReadTokens
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

