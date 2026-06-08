package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/protocol"
)

func TestRenderInfoView_ProcessHoverStrikethrough(t *testing.T) {
	rp := RightPanel{}
	s := NewStyles(true)
	info := RightPanelInfoView{
		Processes: []protocol.BackgroundProcessItem{{
			ID:        "proc-1",
			PID:       4242,
			Command:   "npm run dev",
			StartedAt: time.Now().Add(-2 * time.Minute).Unix(),
			Running:   true,
		}},
		HoverProcessID: "proc-1",
	}
	sections, idx := rp.renderInfoView(panelWidth-4, info, s)
	raw := strings.Join(joinInfoSections(sections), "\n")
	plain := stripANSI(raw)
	if idx["proc-1"] < 0 {
		t.Fatalf("expected process line index, got %v", idx)
	}
	if !strings.Contains(plain, "npm run dev") {
		t.Fatalf("expected command in view, got %q", plain)
	}
	// Lipgloss bundles strikethrough (SGR 9) with other attrs in one sequence.
	if !strings.Contains(raw, ";9m") {
		t.Fatalf("expected strikethrough on hover, got %q", plain)
	}
	if !strings.Contains(raw, "58;42;42") {
		t.Fatalf("expected hover background on process row, got %q", plain)
	}
}

func TestRightPanel_ProcessIDAtContentLine(t *testing.T) {
	rp := RightPanel{processLineIdx: map[string]int{"proc-2": 5}}
	if id, ok := rp.ProcessIDAtContentLine(5); !ok || id != "proc-2" {
		t.Fatalf("ProcessIDAtContentLine(5) = %q, %v", id, ok)
	}
	if _, ok := rp.ProcessIDAtContentLine(4); ok {
		t.Fatal("expected no match on line 4")
	}
}
