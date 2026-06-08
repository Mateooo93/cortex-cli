package ui

import (
	"strings"
	"testing"

	"github.com/Mateooo93/cortex-cli/internal/config"
	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
)

func TestActiveModelSpec_PrefersLiveSessionModel(t *testing.T) {
	cfg := cortexconfig.Default()
	cfg.DefaultModel = "codex/gpt-5.5"
	m := &Model{
		cortexCfg: cfg,
		sessions: []*SessionState{{
			modelName: "xai-sub/grok-build",
		}},
		selectedSession: 0,
	}
	got := m.activeModelSpec(m.currentSession())
	if got != "xai-sub/grok-build" {
		t.Fatalf("activeModelSpec = %q, want xai-sub/grok-build", got)
	}
}

func TestBuildStatusBarInfo_UsesLiveSessionModel(t *testing.T) {
	cfg := cortexconfig.Default()
	cfg.DefaultModel = "codex/gpt-5.5"
	m := &Model{
		cortexCfg: cfg,
		sessions: []*SessionState{{
			modelName: "xai-sub/grok-build",
		}},
		selectedSession: 0,
	}
	info := m.buildStatusBarInfo(m.currentSession())
	if !strings.Contains(info.ModelName, "Grok") {
		t.Fatalf("status bar model = %q, want Grok display name", info.ModelName)
	}
	if info.ProviderTag != "xai-sub" {
		t.Fatalf("provider tag = %q, want xai-sub", info.ProviderTag)
	}
}

func TestBuildRightPanelInfoView_MatchesStatusBarModel(t *testing.T) {
	cfg := cortexconfig.Default()
	cfg.DefaultModel = "codex/gpt-5.5"
	sess := &SessionState{
		modelName: "xai-sub/grok-build",
	}
	m := &Model{
		cortexCfg:       cfg,
		cfg:             &config.Config{},
		sessions:        []*SessionState{sess},
		selectedSession: 0,
	}
	info := m.buildRightPanelInfoView(sess)
	bar := m.buildStatusBarInfo(sess)
	if info.ModelName != bar.ModelName {
		t.Fatalf("panel model %q != status bar model %q", info.ModelName, bar.ModelName)
	}
}