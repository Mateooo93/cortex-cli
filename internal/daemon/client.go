// Package daemon provides a SessionClient that wraps the in-process
// session. It exposes the same surface the (forked) vix UI calls into,
// so the UI code in internal/ui/ doesn't have to change. The "socket"
// path is unused — the client talks directly to the session.
package daemon

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/protocol"
	"github.com/Mateooo93/cortex-cli/internal/provider"
	"github.com/Mateooo93/cortex-cli/internal/session"
	"github.com/google/uuid"
)

// Client is a stub for the daemon ping client. Always returns
// "connected" in our in-process model.
type Client struct {
	authToken string
}

// NewClient creates a new Client. socketPath is ignored.
func NewClient(_ string) *Client { return &Client{} }

// SetAuthToken is a no-op stub.
func (c *Client) SetAuthToken(t string) { c.authToken = t }

// Ping always returns true (in-process).
func (c *Client) Ping() bool { return true }

// SessionClient is the in-process session wrapped to expose the same
// methods the (forked) vix UI calls.
type SessionClient struct {
	mu sync.Mutex
	// The wrapped session
	sess *session.Session
	// Synthetic socket path (unused)
	socketPath string
	// Session ID for the in-process model
	id string
	// Auth token (unused)
	authToken string
	// Started-at time (matches the vix SessionClient.StartedAt() shape)
	startedAt time.Time
	// One-shot channels for pending user input
	confirmCh     chan confirmReq
	userAnswerCh  chan userAnswerReq
	planActionCh chan planActionReq
	setModelCh   chan string
	inputCh       chan inputReq
	trimCh        chan int
	// Listeners for events
	listeners []chan protocol.SessionEvent
	// Closed
	closed bool
}

type confirmReq struct {
	Approved    bool
	PersistDirs bool
}
type userAnswerReq struct {
	Answer  string
	Text    string
	Answers map[string]string
}
type planActionReq struct {
	Action string
	Text   string
}
type inputReq struct {
	Text        string
	Attachments []protocol.Attachment
}

// NewSessionClient creates a new in-process SessionClient.
func NewSessionClient(_ string) *SessionClient {
	return &SessionClient{
		id:            uuid.NewString(),
		startedAt:     time.Now(),
		inputCh:       make(chan inputReq, 1),
		confirmCh:     make(chan confirmReq, 1),
		userAnswerCh:  make(chan userAnswerReq, 1),
		planActionCh: make(chan planActionReq, 1),
		setModelCh:   make(chan string, 1),
		trimCh:        make(chan int, 1),
	}
}

// SetAuthToken is a no-op stub.
func (c *SessionClient) SetAuthToken(t string) { c.authToken = t }

// SessionID returns the client's session ID.
func (c *SessionClient) SessionID() string { return c.id }

// StartedAt returns the time the session was created.
func (c *SessionClient) StartedAt() time.Time { return c.startedAt }

// ConnectFork attaches a session forked from a parent session. Currently
// the in-process model doesn't track parent/child relationships, so this
// behaves like Connect().
func (c *SessionClient) ConnectFork(workdir, configDir, model string, _ bool, _ bool, _ bool, _ bool, _ string, _ int) error {
	return c.Connect(workdir, configDir, model, false, true, true, false)
}

// Connect attaches a session to this client and starts forwarding events.
func (c *SessionClient) Connect(workdir, configDir, model string, _ bool, _ bool, _ bool, _ bool) error {
	cfg := cortexconfigLoader()
	if cfg == nil {
		return errors.New("daemon: config loader not set (call SetGlobalConfigLoader)")
	}
	// Verify the model exists; fall back to default if empty.
	if model == "" {
		model = cfg.DefaultModel
	}
	if _, _, err := cfg.GetModel(model); err != nil {
		// Try the default model as a last resort
		if _, _, err2 := cfg.GetModel(cfg.DefaultModel); err2 == nil {
			model = cfg.DefaultModel
		} else {
			return err
		}
	}
	sess, err := session.New(session.Config{
		CortexCfg:   cfg,
		Workdir:     workdir,
		ConfigDir:   configDir,
		ActiveModel: model,
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.sess = sess
	c.mu.Unlock()

	return nil
}

// forwardEvents pumps the session's events to all listeners.
func (c *SessionClient) forwardEvents() {
	if c.sess == nil {
		return
	}
	for ev := range c.sess.Events() {
		c.mu.Lock()
		listeners := append([]chan protocol.SessionEvent{}, c.listeners...)
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}
		for _, ch := range listeners {
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

// Events returns the session's event channel. The TUI can drain this
// directly without going through the listener pump.
func (c *SessionClient) Events() <-chan protocol.SessionEvent {
	if c.sess == nil {
		return nil
	}
	return c.sess.Events()
}

// SetConfigLoader lets main inject a global config loader (since the
// daemon package is otherwise a stub).
var cortexconfigLoader = func() *cortexconfig.Config {
	return cortexconfig.Default()
}

// SetGlobalConfigLoader replaces the stub loader. Call once from main.
func SetGlobalConfigLoader(f func() *cortexconfig.Config) {
	cortexconfigLoader = f
}

// ReadEvent blocks until an event is available. The UI calls this in a
// goroutine via startSessionEventLoop.
func (c *SessionClient) ReadEvent() (protocol.SessionEvent, error) {
	if c.sess == nil {
		return protocol.SessionEvent{}, nil
	}
	ev, ok := <-c.sess.Events()
	if !ok {
		return protocol.SessionEvent{}, errClosed
	}
	if ev.Type != "" && !strings.HasPrefix(ev.Type, "event.") {
		ev.Type = "event." + ev.Type
	}
	return ev, nil
}

var errClosed = &closedErr{}

type closedErr struct{}

func (c *closedErr) Error() string { return "session closed" }

// SendInput submits a user message to the model.
func (c *SessionClient) SendInput(text string, attachments []protocol.Attachment) error {
	if c.sess == nil {
		return errClosed
	}
	c.sess.Send(text, attachments)
	return nil
}

// SendSetModel switches the model.
func (c *SessionClient) SendSetModel(name string) error {
	if c.sess == nil {
		return errClosed
	}
	return c.sess.SetActiveModel(name)
}

// SendRestoreHistory seeds the session's conversation history
// with the given messages. Used to restore a session's chat
// scrollback across a daemon reconnect.
func (c *SessionClient) SendRestoreHistory(history []provider.Message) error {
	if c.sess == nil {
		return errClosed
	}
	c.sess.RestoreHistory(history)
	return nil
}

// SendCancel asks the running turn to stop.
func (c *SessionClient) SendCancel() {
	if c.sess != nil {
		c.sess.SendCancel()
	}
}

// SendCancelAfterEdit asks the running turn to stop, but only after
// the in-flight tool call (if any) finishes. See Session.SendCancelAfterEdit.
func (c *SessionClient) SendCancelAfterEdit() {
	if c.sess != nil {
		c.sess.SendCancelAfterEdit()
	}
}

// SendSteer injects a mid-turn redirect message (Pi-style steering).
// If a turn is active it will be processed after the current tool batch
// (pair with SendCancelAfterEdit for "finish this edit then do X").
// See Session.Steer.
func (c *SessionClient) SendSteer(text string) {
	if c.sess != nil {
		c.sess.Steer(text)
	}
}

// SendClose shuts the session.
func (c *SessionClient) SendClose() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	if c.sess != nil {
		c.sess.SendClose()
	}
}

// Close mirrors SendClose (some callers use the name Close).
func (c *SessionClient) Close() { c.SendClose() }

// SendConfirm answers a pending tool-confirm request. (Currently a no-op
// because we don't gate tool calls in the in-process model — all
// permissions come from the user config.)
func (c *SessionClient) SendConfirm(approved, persistDirs bool) error {
	select {
	case c.confirmCh <- confirmReq{Approved: approved, PersistDirs: persistDirs}:
	default:
	}
	return nil
}

// SendUserAnswer answers a single user question. It
// forwards the answer to the in-process session, which is
// currently blocked inside handleAskUserQuestion waiting
// for the user to pick an option in the question panel.
// The session drops the answer if no question is pending
// (channel full or panel already closed).
func (c *SessionClient) SendUserAnswer(answer, text string) error {
	if c.sess != nil {
		c.sess.SendUserAnswer(answer, text)
		return nil
	}
	// Fallback: nobody is listening; drop silently. (We
	// keep the channel send as a defensive no-op in case
	// a future caller wants the buffered behaviour.)
	select {
	case c.userAnswerCh <- userAnswerReq{Answer: answer, Text: text}:
	default:
	}
	return nil
}

// SendUserAnswerBatch answers multiple questions at once.
func (c *SessionClient) SendUserAnswerBatch(answers map[string]string) error {
	if c.sess != nil {
		c.sess.SendUserAnswerBatch(answers)
		return nil
	}
	select {
	case c.userAnswerCh <- userAnswerReq{Answers: answers}:
	default:
	}
	return nil
}

// SendPlanAction answers a plan-review prompt.
func (c *SessionClient) SendPlanAction(action, text string) error {
	select {
	case c.planActionCh <- planActionReq{Action: action, Text: text}:
	default:
	}
	return nil
}

// SendStopBackgroundProcess stops a background shell process by ID.
func (c *SessionClient) SendStopBackgroundProcess(processID string) error {
	if c.sess == nil {
		return errors.New("daemon: no session")
	}
	return c.sess.StopBackgroundProcess(processID)
}

// SendTrim trims the conversation history to the given turn index.
// (No-op in our in-process model — kept for API parity.)
func (c *SessionClient) SendTrim(turnIdx int) error {
	select {
	case c.trimCh <- turnIdx:
	default:
	}
	return nil
}
