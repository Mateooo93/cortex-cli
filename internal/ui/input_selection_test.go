package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestInputSelection_MouseDrag(t *testing.T) {
	setupPersistDir(t)
	m := NewModel(&config.Config{}, cortexconfig.Default(), nil, false, "", false, false)
	m.width = 80
	m.height = 30
	m.activeTab = TabKindChat
	sess := m.currentSession()
	sess.input.SetWidth(m.width - 4)
	sess.input.SetValue("hello world")
	sess.input.SetHeight(1)

	top, _, left, _, ok := m.inputInnerBounds()
	if !ok {
		t.Fatal("expected input inner bounds")
	}

	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: left + 1, Y: top})
	m = updated.(Model)
	if !m.currentSession().inputSel.active {
		t.Fatal("expected input selection after click")
	}
	if m.currentSession().chatSel.active {
		t.Fatal("chat selection should be cleared")
	}

	updated, _ = m.Update(tea.MouseMotionMsg{X: left + 6, Y: top})
	m = updated.(Model)
	if m.currentSession().inputSel.endX < 4 {
		t.Fatalf("expected drag to extend input selection, endX=%d", m.currentSession().inputSel.endX)
	}
}

func TestCtrlCCopiesInputSelection(t *testing.T) {
	in := newInput()
	in.SetWidth(76)
	in.SetValue("hello world")
	m := Model{
		width:     80,
		height:    30,
		activeTab: TabKindChat,
		mdRenderer: NewMarkdownRenderer(74, true, lipgloss.NewStyle()),
		sessions: []*SessionState{{
			input: in,
			inputSel: chatSelection{
				active:     true,
				anchorLine: 0,
				anchorX:    0,
				endLine:    0,
				endX:       4,
			},
		}},
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	out := updated.(Model)
	if out.state == StateQuitConfirm {
		t.Fatal("ctrl+c with input selection should copy, not open quit confirm")
	}
}

func TestInputContentInnerWidthMatchesBox(t *testing.T) {
	if got := inputContentInnerWidth(80); got != 76 {
		t.Fatalf("inner width = %d, want 76", got)
	}
}

func TestRenderInputView_HighlightsSelection(t *testing.T) {
	in := newInput()
	in.SetWidth(76)
	in.SetValue("select me")
	m := Model{width: 80, sessions: []*SessionState{{input: in}}}
	sess := m.currentSession()
	sess.inputSel = chatSelection{active: true, anchorLine: 0, anchorX: 0, endLine: 0, endX: 5}
	rendered := m.renderInputView(sess)
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected selection highlight in rendered input, got %q", rendered)
	}
}