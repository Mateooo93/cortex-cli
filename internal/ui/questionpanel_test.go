package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func TestQuestionPanel_AppendsCustomRichOption(t *testing.T) {
	qp := NewQuestionPanel()
	s := NewStyles(true)
	md := NewMarkdownRenderer(80, true, s.CodeBoxBorderStyle)
	event := protocol.EventUserQuestion{
		Question: "Pick one",
		Category: "Choice",
		RichOptions: []protocol.EventQuestionOption{
			{Title: "A", Description: "first"},
			{Title: "B", Description: "second"},
		},
	}
	qp.Open(event, 80, md)

	tab := qp.tabs[0]
	if len(tab.richOptions) != 3 {
		t.Fatalf("expected 3 rich options (2 + custom), got %d", len(tab.richOptions))
	}
	last := tab.richOptions[len(tab.richOptions)-1]
	if !last.HasUserInput || last.Title != "Type something." {
		t.Fatalf("expected custom text option, got %+v", last)
	}
}

func TestQuestionPanel_BatchShowsAllQuestions(t *testing.T) {
	qp := NewQuestionPanel()
	s := NewStyles(true)
	md := NewMarkdownRenderer(80, true, s.CodeBoxBorderStyle)
	event := protocol.EventUserQuestion{
		Questions: []protocol.QuestionDef{
			{ID: "scope", Category: "Scope", Question: "How big?", Options: []string{"Small", "Large"}},
			{ID: "style", Category: "Style", Question: "Which look?", Options: []string{"Minimal", "Bold"}},
		},
	}
	qp.Open(event, 80, md)

	if len(qp.tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(qp.tabs))
	}
	if !qp.isMultiTab() {
		t.Fatal("expected multi-tab batch mode")
	}

	rendered := stripANSI(qp.Render(s, true, md))
	for _, want := range []string{"Scope", "Style", "How big", "Which look"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered panel missing %q:\n%s", want, rendered)
		}
	}
}