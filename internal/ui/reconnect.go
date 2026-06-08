package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/daemon"
)

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

// modelForReconnect returns the model name to use when reconnecting sess.
func (m *Model) modelForReconnect(sess *SessionState) string {
	if sess == nil {
		return m.activeModelForNewSession()
	}
	if sess.modelName != "" {
		return sess.modelName
	}
	return m.activeModelForNewSession()
}

// reconnectSession starts a background reconnect for the given session.
func (m *Model) reconnectSession(sess *SessionState, forceInit bool) tea.Cmd {
	if sess == nil {
		return nil
	}
	return attemptReconnect(
		m.socketPath, m.cwd, m.cfg.ConfigDir,
		m.modelForReconnect(sess), m.authToken,
		forceInit, m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess,
		sess.daemonSessionID,
	)
}

// connectForkSession starts a forked daemon session for newSess.
func (m *Model) connectForkSession(newSess *SessionState, forkSessionID string, forkTurnIdx int, model string) tea.Cmd {
	if newSess == nil {
		return nil
	}
	return connectFork(
		m.socketPath, m.cwd, m.cfg.ConfigDir, model, m.authToken,
		m.enableAutomaticWritePermission, m.enableAutomaticDirectoryAccess,
		forkSessionID, forkTurnIdx, newSess.daemonSessionID,
	)
}
