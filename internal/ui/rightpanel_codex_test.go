package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestRightPanel_CodexAutoTriggersOAuth verifies that pressing Enter
// on a codex model in the picker immediately returns the
// rpActionCodexSignIn action — i.e. the user does NOT have to go
// through a "press Enter again to sign in" intermediate panel.
//
// This is the canonical flow documented by OpenAI: the codex CLI
// opens the browser as soon as you pick a codex model, no extra
// step. The chatgpt.com sign-in page is the entire user-facing UI
// for the auth step.
func TestRightPanel_CodexAutoTriggersOAuth(t *testing.T) {
	// Find a codex model in the catalogue.
	var codexIdx int
	var codexSpec string
	for i, m := range AvailableModels {
		if m.Provider == "codex" {
			codexIdx = i
			codexSpec = m.Spec
			break
		}
	}
	if codexSpec == "" {
		t.Fatal("no codex model in AvailableModels")
	}

	rp := RightPanel{}
	rp.modelSel = codexIdx

	action, payload := rp.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != rpActionCodexSignIn {
		t.Errorf("action on codex enter = %d, want rpActionCodexSignIn (%d)", action, rpActionCodexSignIn)
	}
	if payload != codexSpec {
		t.Errorf("payload = %q, want %q (the codex model spec)", payload, codexSpec)
	}
}

// TestRightPanel_CodexNeverReturnsNeedKey is the strongest invariant
// for the codex auth path: the user should NEVER be asked to paste
// an API key. If this test fails, the user will see the "paste your
// OpenAI API key" prompt — which is wrong because codex uses their
// ChatGPT subscription, not paid API credits.
func TestRightPanel_CodexNeverReturnsNeedKey(t *testing.T) {
	for i, m := range AvailableModels {
		if m.Provider != "codex" {
			continue
		}
		rp := RightPanel{}
		rp.modelSel = i
		action, _ := rp.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if action == rpActionNeedKey {
			t.Errorf("codex model %q returned rpActionNeedKey — must use OAuth, not API key", m.Spec)
		}
	}
}

// TestRightPanel_CodexNeverReturnsModelSelectedWithoutKey checks the
// corollary: a codex model is NEVER "model selected" just because
// the keychain happens to contain a token. The codex path is
// always OAuth-first.
func TestRightPanel_CodexNeverReturnsModelSelectedDirectly(t *testing.T) {
	for i, m := range AvailableModels {
		if m.Provider != "codex" {
			continue
		}
		rp := RightPanel{}
		rp.modelSel = i
		action, _ := rp.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if action == rpActionModelSelected {
			t.Errorf("codex model %q returned rpActionModelSelected on first enter — must always go through OAuth", m.Spec)
		}
	}
}

func TestRightPanel_XaiSubAutoTriggersOAuth(t *testing.T) {
	var idx int
	var spec string
	for i, m := range AvailableModels {
		if m.Provider == "xai-sub" {
			idx = i
			spec = m.Spec
			break
		}
	}
	if spec == "" {
		t.Fatal("no xai-sub model in AvailableModels")
	}
	rp := RightPanel{}
	rp.modelSel = idx
	action, payload := rp.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if action != rpActionCodexSignIn {
		t.Errorf("action = %d, want rpActionCodexSignIn", action)
	}
	if payload != spec {
		t.Errorf("payload = %q, want %q", payload, spec)
	}
	if rp.oauthSignInProvider != "xai-sub" {
		t.Errorf("oauthSignInProvider = %q, want xai-sub", rp.oauthSignInProvider)
	}
}

func TestRightPanel_XaiSubNeverReturnsNeedKey(t *testing.T) {
	for i, m := range AvailableModels {
		if m.Provider != "xai-sub" {
			continue
		}
		rp := RightPanel{}
		rp.modelSel = i
		action, _ := rp.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if action == rpActionNeedKey {
			t.Errorf("xai-sub model %q returned rpActionNeedKey — must use OAuth subscription", m.Spec)
		}
	}
}

// TestRightPanel_NonCodexNeedsAPIKey confirms the inverse: picking a
// normal API-key provider (e.g. openai) does NOT trigger the codex
// OAuth path.
func TestRightPanel_NonCodexNeedsAPIKey(t *testing.T) {
	for i, m := range AvailableModels {
		if m.Provider == "codex" || m.Provider == "xai-sub" {
			continue
		}
		rp := RightPanel{}
		rp.modelSel = i
		action, payload := rp.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		// For a non-codex provider, the picker must return one of:
		//   - rpActionModelSelected (if a key is already stored)
		//   - rpActionNeedKey       (if not — user has to paste a key)
		// It must NEVER silently fire rpActionCodexSignIn.
		if action == rpActionCodexSignIn {
			t.Errorf("non-codex model %q fired rpActionCodexSignIn", m.Spec)
		}
		switch action {
		case rpActionModelSelected:
			if payload != m.Spec {
				t.Errorf("non-codex: payload = %q, want %q", payload, m.Spec)
			}
		case rpActionNeedKey:
			if !strings.HasPrefix(payload, m.Provider+":") {
				t.Errorf("non-codex: need-key payload = %q, want %q-prefix", payload, m.Provider)
			}
		default:
			t.Errorf("non-codex model %q: unexpected action %d", m.Spec, action)
		}
	}
}
