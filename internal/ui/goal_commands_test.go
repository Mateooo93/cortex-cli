package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	"github.com/Mateooo93/cortex-cli/internal/daemon"
)

func wireTestSessionClient(t *testing.T, sess *SessionState) {
	t.Helper()
	client := daemon.NewSessionClient("")
	if err := client.Connect("/tmp", "", "anthropic/claude-sonnet-4.5", false, true, true, false); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { client.SendClose() })
	sess.client = client
	sess.daemonSessionID = client.SessionID()
	sess.reconnecting = false
}

func TestSlashCommandFromLine_GoalWithCondition(t *testing.T) {
	name, args, ok := slashCommandFromLine("/goal all tests pass")
	if !ok {
		t.Fatal("expected ok")
	}
	if name != "goal" {
		t.Fatalf("name = %q, want goal", name)
	}
	if args != "all tests pass" {
		t.Fatalf("args = %q, want 'all tests pass'", args)
	}
}

func TestSlashCommandArgs_StripsCommandPrefix(t *testing.T) {
	got := slashCommandArgs("/goal fix the failing test", "goal")
	if got != "fix the failing test" {
		t.Fatalf("got %q", got)
	}
	if slashCommandArgs("fix the failing test", "goal") != "fix the failing test" {
		t.Fatal("expected bare args to pass through")
	}
}

func TestTryDispatchSlashInput_GoalSetsStreaming(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	wireTestSessionClient(t, sess)

	cmds, handled := m.tryDispatchSlashInput(sess, "/goal npm test exits 0")
	if !handled {
		t.Fatal("expected /goal line to be handled as slash command")
	}
	if len(cmds) == 0 {
		t.Fatal("expected cmds from goal dispatch")
	}
	if sess.agentState != StateStreaming {
		t.Fatalf("agentState = %v, want StateStreaming", sess.agentState)
	}
	foundUser := false
	for _, msg := range sess.chatMessages {
		if strings.Contains(msg.Text, "npm test exits 0") {
			foundUser = true
		}
	}
	if !foundUser {
		t.Fatalf("expected goal condition in chat, got %d messages", len(sess.chatMessages))
	}
}

func TestHandleGoalCommand_FromSlashMenuBareGoal(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]

	cmds := m.handleCommandAction("slash_goal", sess, "/goal")
	if len(cmds) != 0 {
		t.Fatalf("bare /goal status query should not start turn, got %d cmds", len(cmds))
	}
	if sess.agentState == StateStreaming {
		t.Fatal("bare /goal should not start streaming")
	}
}

func TestHandleEnter_DispatchesGoalWithCondition(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	wireTestSessionClient(t, sess)
	sess.agentState = StateWaitingForInput
	sess.input.SetValue("/goal make all tests pass")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(Model)
	sess = um.sessions[0]
	if sess.agentState != StateStreaming {
		t.Fatalf("agentState = %v, want StateStreaming after /goal <condition>", sess.agentState)
	}
	if sess.input.Value() != "" {
		t.Fatalf("input should be cleared, got %q", sess.input.Value())
	}
}