package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestEffortPicker_OpenPreselectsCurrentLevel(t *testing.T) {
	var ep EffortPicker
	ep.Open("ultracode")
	if ep.Selected() != "ultracode" {
		t.Fatalf("selected = %q, want ultracode", ep.Selected())
	}
	ep.Open("")
	if ep.Selected() != "high" {
		t.Fatalf("empty level should default to high, got %q", ep.Selected())
	}
}

func TestEffortPicker_ViewShowsAllLevels(t *testing.T) {
	var ep EffortPicker
	ep.Open("medium")
	view := ep.View(NewStyles(true))
	plain := stripANSI(view)
	for _, want := range []string{"Low", "Medium", "High", "Ultracode", "Reasoning Effort"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q in picker view:\n%s", want, view)
		}
	}
}

func TestOpenEffortPicker_Action(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	sess.effortLevel = "low"

	cmds := m.handleCommandAction("open_effort_picker", sess)
	if len(cmds) != 0 {
		t.Fatalf("open_effort_picker should not return cmds, got %d", len(cmds))
	}
	if !m.effortPicker.IsVisible() {
		t.Fatal("effort picker should be visible")
	}
	if m.effortPicker.Selected() != "low" {
		t.Fatalf("picker selected = %q, want low", m.effortPicker.Selected())
	}
}

func TestEffortPicker_EnterAppliesLevel(t *testing.T) {
	setupPersistDir(t)
	cfg := &config.Config{}
	cortexCfg := cortexconfig.Default()
	m := NewModel(cfg, cortexCfg, nil, false, "", false, false)
	sess := m.sessions[0]
	m.effortPicker.Open("high")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(Model)
	if um.effortPicker.IsVisible() {
		t.Fatal("picker should close after Enter")
	}
	if sess.effortLevel != "high" {
		t.Fatalf("effortLevel = %q, want high", sess.effortLevel)
	}
}

func TestSlashMenu_NoSkillsCommand(t *testing.T) {
	for _, cmd := range slashCommands {
		if cmd.Name == "skills" {
			t.Fatal("/skills should be removed from slash menu")
		}
	}
}