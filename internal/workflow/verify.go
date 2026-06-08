package workflow

import (
	"context"
	"fmt"
	"strings"
)

// Verdict is the outcome of adversarial verification for one finding.
type Verdict struct {
	IsReal       bool   `json:"isReal"`       // true when the finding survives verification
	Finding      string `json:"finding"`      // the original finding text
	VotesFor     int    `json:"votesFor"`     // verifiers that confirmed
	VotesAgainst int    `json:"votesAgainst"` // verifiers that refuted
	Reasons      string `json:"reasons"`      // concatenated verifier reasons
}

// VerifyConfig controls adversarial verification behavior.
type VerifyConfig struct {
	NumVerifiers int      // how many independent skeptics to spawn (default 3)
	Threshold    int      // how many must refute to kill a finding (default 2)
	Lenses       []string // distinct perspectives for diversity (optional)
	// When Lenses is set, each verifier gets a different lens instead
	// of identical refutation prompts. This catches failure modes that
	// redundancy misses (security, correctness, perf, reproducibility).
}

// DefaultVerifyConfig returns a conservative 3-voter config.
func DefaultVerifyConfig() VerifyConfig {
	return VerifyConfig{
		NumVerifiers: 3,
		Threshold:    2,
	}
}

// buildRefutePrompt creates a prompt asking a verifier to refute a finding.
func buildRefutePrompt(finding string, lens string) string {
	if lens != "" {
		return fmt.Sprintf(
			`You are an adversarial verifier. Your job is to check whether the following finding is real.

			Look at it specifically through the lens of: %s.

			Finding: %s

			Be skeptical. If the finding cannot be independently confirmed or has flaws, refute it.
			Respond in this exact format:
			VERDICT: <CONFIRMED or REFUTED>
			REASON: <one sentence>`, lens, finding)
	}
	return fmt.Sprintf(
		`You are an adversarial verifier. Your job is to check whether the following finding is real.

		Finding: %s

		Be skeptical. If the finding cannot be independently confirmed or has flaws, refute it.
		Respond in this exact format:
		VERDICT: <CONFIRMED or REFUTED>
		REASON: <one sentence>`, finding)
}

// parseVerdict extracts the verifier's binary decision.
func parseVerdict(response string) (confirmed bool, reason string) {
	lower := strings.ToLower(response)
	if strings.Contains(lower, "verdict: confirmed") || strings.Contains(lower, "verdict:confirmed") {
		confirmed = true
	}
	if idx := strings.Index(lower, "reason:"); idx >= 0 {
		reason = strings.TrimSpace(response[idx+7:])
		if nl := strings.IndexAny(reason, "\n\r"); nl >= 0 {
			reason = strings.TrimSpace(reason[:nl])
		}
	}
	return
}

// callVerifier invokes a single verifier agent through the engine.
// This is a helper used by AdversarialVerify.
func (e *Engine) callVerifier(ctx context.Context, prompt string) (string, error) {
	// Use the reviewer role for verification since it's the
	// closest match to adversarial checking.
	role := findRole("reviewer")
	if role == nil {
		role = &BuiltinRoles[0] // fallback to planner
	}
	// Create a minimal throwaway workflow for the call context
	wf := &Workflow{ID: "verify-tmp", Goal: prompt}
	return e.callRole(ctx, *role, prompt, wf)
}

// AdversarialVerify runs N independent skeptics against each finding
// and returns the set that survive (≥threshold confirmations).
//
//	findings := []string{"auth middleware missing in /api/admin"}
//	survivors := engine.AdversarialVerify(ctx, findings, DefaultVerifyConfig())
//
// This implements the "adversarial verification" quality pattern
// from Claude Code's dynamic workflows. Combine with perspective-
// diverse lenses for stronger verification:
//
//	cfg := VerifyConfig{
//	    NumVerifiers: 3,
//	    Threshold:    2,
//	    Lenses: []string{"correctness", "security", "reproducibility"},
//	}
func (e *Engine) AdversarialVerify(ctx context.Context, findings []string, cfg VerifyConfig) []Verdict {
	if len(findings) == 0 {
		return nil
	}
	if cfg.NumVerifiers <= 0 {
		cfg.NumVerifiers = 3
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = 2
	}

	type indexedVerdict struct {
		idx int
		v   Verdict
	}
	results := make(chan indexedVerdict, len(findings))

	for i, finding := range findings {
		go func(idx int, f string) {
			var votesFor, votesAgainst int
			var reasons []string

			for v := 0; v < cfg.NumVerifiers; v++ {
				lens := ""
				if v < len(cfg.Lenses) {
					lens = cfg.Lenses[v]
				}
				prompt := buildRefutePrompt(f, lens)
				resp, err := e.callVerifier(ctx, prompt)
				if err != nil {
					votesAgainst++ // assume refuted on error
					reasons = append(reasons, fmt.Sprintf("V%d: error: %v", v+1, err))
					continue
				}
				confirmed, reason := parseVerdict(resp)
				if confirmed {
					votesFor++
				} else {
					votesAgainst++
				}
				reasons = append(reasons, fmt.Sprintf("V%d: %s", v+1, reason))
			}

			isReal := votesFor >= cfg.Threshold
			results <- indexedVerdict{
				idx: idx,
				v: Verdict{
					IsReal:       isReal,
					Finding:      f,
					VotesFor:     votesFor,
					VotesAgainst: votesAgainst,
					Reasons:      strings.Join(reasons, "; "),
				},
			}
		}(i, finding)
	}

	out := make([]Verdict, len(findings))
	for range findings {
		r := <-results
		out[r.idx] = r.v
	}
	return out
}
