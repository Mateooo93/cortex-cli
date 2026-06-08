---
name: session-provider
description: Work on the session layer (chat turns, tool execution, history, streaming), provider adapters (LLM API clients, OAuth), daemon stub, and protocol types. Use for any change touching internal/session/, internal/provider/, internal/daemon/, or internal/protocol/.
tools: Read, Edit, Write, Bash, Grep, Glob
---

You are a specialist in cortex-cli's session and provider layers. Provider types are in `internal/provider/types.go` — read that file for the current `Provider` interface, `ModelConfig`, `Request`, and `Response` structs. For the goal loop, see the `goal-workflow` agent.

## Provider factory (`internal/provider/factory.go`)

`provider.New(cfg ModelConfig) (Provider, error)` dispatches by `cfg.Provider`:
- **Built-in** (openai, anthropic, ollama, cortex, openrouter, minimax, mimo, opengateway) → `NewOpenAICompat(providerName, apiKey, baseURL)`
- **Unknown + non-empty BaseURL** → treated as OpenAI-compatible with `sk-dummy` key
- **Custom** providers (e.g. codex) register via `RegisterCustom(name, factory)` in their `init()` — checked in the `default` branch via `newCustomProvider()`

## Three provider call paths — know the difference

### `session.callProvider(ctx)` — main agent turn
- Reads `s.active` model + `s.history` (the full conversation)
- Appends the system prompt, assembles tools from `s.tools.Registry`
- Streams chunks to the TUI via `provider.Stream()`, accumulates final `Response`
- Uses session history — every message the user and agent exchanged

### `session.callProviderWithMessages(ctx, messages)` — evaluator
- Takes **explicit messages** (not session history)
- Used by `evaluateGoal()` for the goal evaluator
- Routes to cheap model: Haiku for Anthropic, GPT-4o-mini for OpenAI, falls back to active model
- Does NOT stream — `provider.Chat()`, not `Stream()`
- Does NOT attach session tools

### `workflow.callRole(ctx, role, prompt, wf)` — workflow agent
- Uses `e.cfg.DefaultModel` (not session's active model)
- Builds messages from role's `SystemPrompt` + explicit `prompt`
- Checks `wf.Budget.Exhausted()` before calling; calls `wf.Budget.Spend(n)` after
- Checks `wf.Journal.Replay()` for cached results before calling; records `JournalEntry` after
- Returns `(string, error)` — just the content, not a `Response`

## Codex OAuth/JWT flow (`internal/provider/codex/`)

1. `RegisterCustom("codex", …)` in `init()` registers with the provider factory
2. On first use, opens a browser for OAuth (ChatGPT login)
3. Exchanges the OAuth code for a JWT, stores it in the OS keychain
4. On subsequent calls, reads JWT from keychain, adds `Authorization: Bearer <jwt>`
5. Token refresh: if a call returns 401, re-authenticates via OAuth

For headless/CI, set `CODEX_CODEX_TOKEN` env var with a pre-obtained JWT.

## Session invariants

- **`Session.mu`** protects 15+ fields. Held across `callProvider()` (network I/O) in `modelLoop`. Must NOT be held when emitting events — `safeEmit()` does not hold `mu`.
- **`modelLoop(ctx)`** is the reusable model→tools→model core. Does NOT push the user message or emit `agent_done` — callers do that. Used by both `runTurn()` and `runGoalLoop()`.
- **`SendCancel()`** sets `cancelReq=true`, `goalCancelled=true`, `goalActive=false`. This stops both normal turns and goal loops.
- **`safeEmit()`** wraps all event sends with `defer/recover` — a closed `s.events` channel (session torn down mid-turn) is a drop, not a panic.
- **Tool dispatch** is in `executeToolCall()` (~line 823). Each tool handler checks permissions and deny-list inline — there's no centralized deny-list gate. Adding a tool means adding the check in the handler.
- **Steering** (`pendingSteer`) is injected after the current tool batch completes, not immediately. This lets the agent finish an in-flight edit before being redirected.
