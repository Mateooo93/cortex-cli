package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestDisplayChatLineCountMatchesInnerBounds(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 80
	m.height = 40
	m.activeTab = TabKindChat
	sess := m.currentSession()
	sess.chatMessages = []ChatMessage{
		{Type: MsgAssistant, Rendered: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD080")).Render(strings.Repeat("a", 120)) + "\n"},
	}
	layout := m.currentLayout()
	lines := m.displayChatLines(sess, layout)
	top, bottom, _, _ := m.chatInnerBounds()
	if len(lines) != bottom-top {
		t.Fatalf("display lines %d != inner rows %d", len(lines), bottom-top)
	}
}