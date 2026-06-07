package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
)

// TestNewSessionHandleEnterSendsMessage verifies the user-
// reported "AI doesn't respond at all in new sessions" bug
// is fixed. We simulate: Ctrl+T creates a new session,
// the reconnect completes, the user types a message, and
// presses Enter. The state must transition to
// StateStreaming and the message must be in the chat
// scrollback. The root cause of the bug was that
// restored-from-disk session placeholders and freshly-
// created Ctrl+T sessions both had empty
// daemonSessionIDs, so when the reconnect completed the
// new client was attached to the first session with
// daemonSessionID == "" (usually a stale restored
// placeholder), not the new session. The new session
// was left with client=nil and every submit went through
// the "Reconnecting to daemon…" branch.
//
// The fix: newSessionState gives each not-yet-connected
// session a unique random placeholder daemonSessionID,
// so findSessionByDaemonID can match the new client
// back to the right session.
func TestNewSessionHandleEnterSendsMessage(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	cortexCfg.EnsureProviderPresets()
	daemon.SetGlobalConfigLoader(func() *cortexconfig.Config { return cortexCfg })

	m := NewModel(cfg, cortexCfg, nil, false, "", true, true)
	m.width = 140
	m.height = 35
	m.activeTab = TabKindChat

	// Sanity: there may be a restored session placeholder
	// (the test runs from a directory with a .cortex/
	// folder). Capture it for later comparison.
	var restoredSess *SessionState
	if len(m.sessions) > 1 {
		restoredSess = m.sessions[1]
	}

	// Append a brand-new session (mimics Ctrl+T).
	sess := newSessionState(m.cfg, nil)
	sess.modelName = "anthropic/claude-sonnet-4.5"
	sess.reconnecting = true
	m.sessions = append(m.sessions, sess)
	m.selectedSession = len(m.sessions) - 1

	// The placeholder daemonSessionIDs of the restored
	// session (if any) and the new session must differ.
	if restoredSess != nil {
		if restoredSess.daemonSessionID == sess.daemonSessionID {
			t.Errorf("placeholder IDs collide: %q", restoredSess.daemonSessionID)
		}
	}
	if !strings.HasPrefix(sess.daemonSessionID, "pending-") {
		t.Errorf("expected pending- prefix, got %q", sess.daemonSessionID)
	}

	// Wire a real client (mimic reconnectSuccessMsg).
	client := daemon.NewSessionClient("")
	if err := client.Connect("/tmp", "", "anthropic/claude-sonnet-4.5", false, true, true, false); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { client.SendClose() })

	// The bug: findSessionByDaemonID returned the wrong
	// session (the restored one) when both shared
	// daemonSessionID == "". The fix gives each session a
	// unique placeholder ID.
	_, target := m.findSessionByDaemonID(sess.daemonSessionID)
	if target != sess {
		t.Fatalf("findSessionByDaemonID returned wrong session; new session was not matchable")
	}
	target.client = client
	target.daemonSessionID = client.SessionID()
	target.reconnecting = false

	// The restored placeholder must NOT have been
	// accidentally wired with the new client.
	if restoredSess != nil && restoredSess.client != nil {
		t.Errorf("restored placeholder was wrongly attached a client (the original bug)")
	}

	// User types and presses Enter.
	sess.input.SetValue("hello")
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// State must be StateStreaming.
	if sess.agentState != StateStreaming {
		t.Errorf("expected StateStreaming, got %v", sess.agentState)
	}
	// User message must be in chat scrollback.
	found := false
	for _, msg := range sess.chatMessages {
		if msg.Type == MsgUser && msg.Text == "hello" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("user message not in chat history. messages: %+v", sess.chatMessages)
	}
}
