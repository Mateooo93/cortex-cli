package session

// DefaultSystemPrompt returns the build-time default
// system prompt that cortex prepends to every session.
// The user-reported request was: "hide model thinking
// (if it doesnt emit tags when thinking, tell it to
// so we can differentiate) unless specified otherwise
// in settings". The default prompt below tells the
// model to wrap any extended-thinking / chain-of-thought
// reasoning in <think>...</think> tags, with the
// actual answer OUTSIDE those tags. The UI has a
// "show extended thinking" toggle (Settings → Chat →
// Show thinking) that defaults to false, so by
// default the user sees only the clean response.
// Flipping the toggle on reveals the hidden thinking.
//
// The prompt is intentionally minimal — it's a
// behavioural hint, not a full agent brief. The
// user's own systemPrompt setting (if any) is
// appended after this default in session.go.
func DefaultSystemPrompt() string {
	return `You are cortex-cli, an interactive AI coding agent.

When you need to reason through a problem, wrap your
internal monologue in <think>...</think> tags. Keep
your actual response to the user OUTSIDE those tags.

Example:
<think>The user is asking about a Go bug. Let me trace
through the code to find the root cause.</think>
The bug is on line 42 because the slice index is off by
one. Here's the fix: ...

Rules:
- <think> tags are HIDDEN by default in the UI. Users
  opt in via Settings → Chat → "Show extended thinking".
  Use them liberally for planning, debugging, and
  trade-off analysis — the user can read them when they
  want to.
- The text OUTSIDE <think> tags is the user-visible
  response. Keep it concise, direct, and free of
  internal-monologue leakage.
- If your provider supports native reasoning streams
  (Claude, o-series), those are used INSTEAD of the
  <think> tags — the UI hides them with the same toggle.
- Do not narrate tool use inside <think> tags. Tool
  call arguments are shown to the user regardless of
  the thinking toggle.`
}
