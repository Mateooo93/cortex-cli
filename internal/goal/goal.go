// Package goal implements the /goal command — an autonomous,
// evaluator-driven agent loop. The user sets a measurable end
// condition; after each turn a small fast model judges whether the
// condition is met. If not, the main agent keeps working.
//
// Architecture (dual-model loop):
//
//	Worker (main model) ──transcript──→ Evaluator (cheap model)
//	       ↑                                  │
//	       └──── feedback (NO + reason) ──────┘
//	                                          │
//	                         YES → clear goal, return control
//
// The evaluator cannot run commands or read files. It judges only
// from what the worker has surfaced in the conversation transcript.
// This prevents the self-assessment bias that plagues single-model
// agent loops.
package goal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Mateooo93/cortex-cli/internal/cortexconfig"
	llmprovider "github.com/Mateooo93/cortex-cli/internal/provider"
)

// Status is the lifecycle state of a goal.
type Status string

const (
	StatusInactive  Status = "inactive"
	StatusActive    Status = "active"
	StatusAchieved  Status = "achieved"
	StatusCleared   Status = "cleared"
	StatusFailed    Status = "failed"
)

// Goal represents one active goal with its execution state.
type Goal struct {
	Condition   string    // the user's goal condition
	Status      Status    // current lifecycle state
	StartedAt   time.Time // when the goal was set
	AchievedAt  time.Time // when the condition was met (if achieved)
	Turns       int       // number of evaluated turns
	TokenSpend  int       // approximate tokens used in goal turns
	LastReason  string    // evaluator's most recent verdict reason
}

// EvaluatorResult is the output of the evaluator model.
type EvaluatorResult struct {
	Met    bool   // true when the condition is satisfied
	Reason string // natural-language explanation
}

// evaluatorPrompt is the system prompt sent to the evaluator model.
const evaluatorPrompt = `You are a goal evaluator. Your job is to determine whether a stated goal condition has been met.

You will receive:
1. A GOAL CONDITION that the worker agent is trying to achieve.
2. A CONVERSATION TRANSCRIPT showing what the worker agent has done.

Rules:
- Judge ONLY from what is explicitly shown in the transcript.
- You CANNOT run commands, read files, or access any external system.
- If the transcript shows clear evidence the condition is satisfied, answer YES.
- If the condition is not yet satisfied or evidence is ambiguous, answer NO.
- Be strict: the worker must demonstrate completion, not just claim it.

Respond in this exact format (no markdown, no preamble):
VERDICT: <YES or NO>
REASON: <one concise sentence explaining your judgment>`

// buildEvaluatorMessages constructs the messages sent to the evaluator.
func buildEvaluatorMessages(condition string, transcript string) []llmprovider.Message {
	userPrompt := fmt.Sprintf(
		"GOAL CONDITION:\n%s\n\nCONVERSATION TRANSCRIPT:\n%s",
		condition, truncateTranscript(transcript, 8000),
	)
	return []llmprovider.Message{
		{Role: "system", Content: evaluatorPrompt},
		{Role: "user", Content: userPrompt},
	}
}

// truncateTranscript keeps the tail of a transcript up to maxChars.
// The most recent work is at the end, which is most relevant for
// judging whether the goal is now met.
func truncateTranscript(transcript string, maxChars int) string {
	if len(transcript) <= maxChars {
		return transcript
	}
	return "...(earlier transcript omitted)...\n" + transcript[len(transcript)-maxChars:]
}

// parseEvaluatorResponse extracts the VERDICT and REASON from the
// evaluator model's response. Tolerates minor formatting variations.
func parseEvaluatorResponse(content string) EvaluatorResult {
	result := EvaluatorResult{}
	lower := strings.ToLower(content)

	// Extract verdict
	if strings.Contains(lower, "verdict: yes") || strings.Contains(lower, "verdict:yes") {
		result.Met = true
	} else if strings.Contains(lower, "verdict: no") || strings.Contains(lower, "verdict:no") {
		result.Met = false
	} else {
		// Fallback: look for YES/NO anywhere
		if strings.Contains(lower, "\nyes") || strings.HasPrefix(lower, "yes") {
			result.Met = true
		}
	}

	// Extract reason
	if idx := strings.Index(lower, "reason:"); idx >= 0 {
		reason := strings.TrimSpace(content[idx+7:])
		// Take just the first line
		if nl := strings.IndexAny(reason, "\n\r"); nl >= 0 {
			reason = strings.TrimSpace(reason[:nl])
		}
		result.Reason = reason
	} else {
		result.Reason = content
		if len(result.Reason) > 200 {
			result.Reason = result.Reason[:200] + "..."
		}
	}

	return result
}

// Manager owns the session-scoped goal state and provides the
// evaluator loop. It is goroutine-safe.
type Manager struct {
	mu   sync.RWMutex
	cfg  *cortexconfig.Config
	goal *Goal
}

// NewManager creates a goal Manager bound to the user's config.
func NewManager(cfg *cortexconfig.Config) *Manager {
	return &Manager{cfg: cfg}
}

// Set replaces any active goal with a new one and returns it.
// The goal starts in StatusActive.
func (m *Manager) Set(condition string) *Goal {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.goal = &Goal{
		Condition: condition,
		Status:    StatusActive,
		StartedAt: time.Now(),
	}
	return m.goal
}

// Clear removes the active goal. If the goal was active, it is
// marked as cleared; if already achieved, it is left as-is.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.goal != nil && m.goal.Status == StatusActive {
		m.goal.Status = StatusCleared
	}
}

// Active returns the current goal, or nil if none is active.
func (m *Manager) Active() *Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.goal == nil || m.goal.Status != StatusActive {
		return nil
	}
	// Return a copy so callers can't mutate
	g := *m.goal
	return &g
}

// State returns a copy of the current goal regardless of status,
// or nil if no goal has ever been set.
func (m *Manager) State() *Goal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.goal == nil {
		return nil
	}
	g := *m.goal
	return &g
}

// Evaluate sends the transcript to the evaluator model and returns
// whether the goal is met. Called after each turn when a goal is
// active.
//
// If the evaluator returns YES, the goal is marked as achieved.
// If NO, the reason is stored for the next turn's guidance.
func (m *Manager) Evaluate(ctx context.Context, transcript string) (EvaluatorResult, error) {
	m.mu.RLock()
	g := m.goal
	m.mu.RUnlock()

	if g == nil || g.Status != StatusActive {
		return EvaluatorResult{Met: true, Reason: "no active goal"}, nil
	}

	result, err := m.callEvaluator(ctx, g.Condition, transcript)
	if err != nil {
		return result, err
	}

	m.mu.Lock()
	if m.goal != nil && m.goal.Status == StatusActive {
		m.goal.Turns++
		m.goal.LastReason = result.Reason
		if result.Met {
			m.goal.Status = StatusAchieved
			m.goal.AchievedAt = time.Now()
		}
	}
	m.mu.Unlock()

	return result, nil
}

// RecordTokens adds to the goal's token spend counter.
func (m *Manager) RecordTokens(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.goal != nil && m.goal.Status == StatusActive {
		m.goal.TokenSpend += n
	}
}

// callEvaluator invokes the cheapest available model to judge
// whether the goal condition is met.
func (m *Manager) callEvaluator(ctx context.Context, condition, transcript string) (EvaluatorResult, error) {
	if m.cfg == nil {
		return EvaluatorResult{}, fmt.Errorf("goal manager has no config")
	}

	// Resolve the evaluator model: prefer Haiku for Anthropic,
	// GPT-4o-mini for OpenAI, or fall back to the default model.
	providerName, modelName, apiKey := m.resolveEvaluatorModel()

	if apiKey == "" {
		return EvaluatorResult{}, fmt.Errorf("no API key available for goal evaluator")
	}

	prov, err := llmprovider.New(llmprovider.ModelConfig{
		Provider: providerName,
		Model:    modelName,
		APIKey:   apiKey,
	})
	if err != nil {
		return EvaluatorResult{}, fmt.Errorf("create evaluator provider: %w", err)
	}

	messages := buildEvaluatorMessages(condition, transcript)
	req := llmprovider.Request{
		Model:    modelName,
		Messages: messages,
	}

	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return EvaluatorResult{}, fmt.Errorf("evaluator call: %w", err)
	}

	return parseEvaluatorResponse(resp.Content), nil
}

// resolveEvaluatorModel picks the cheapest available model for
// goal evaluation. It walks the configured models and returns the
// first matching cheap evaluator, or the default model.
func (m *Manager) resolveEvaluatorModel() (providerName, modelName, apiKey string) {
	// Try the default model's provider with a cheap model
	_, mc, err := m.cfg.GetModel(m.cfg.DefaultModel)
	if err == nil && mc != nil {
		apiKey = mc.APIKey
		if apiKey == "" {
			if env := cortexconfig.ProviderEnvVar(mc.Provider); env != "" {
				apiKey = getEnv(env)
			}
		}
		if apiKey != "" {
			switch strings.ToLower(mc.Provider) {
			case "anthropic":
				return mc.Provider, "claude-haiku-4-5-20251001", apiKey
			case "openai":
				return mc.Provider, "gpt-4o-mini", apiKey
			default:
				return mc.Provider, mc.Model, apiKey
			}
		}
	}
	return "", "", ""
}

// getEnv is a shim for os.Getenv so the evaluator doesn't import "os".
var getEnv = func(name string) string { return "" }
