# Cortex CLI (Go fork of Vix)

This is a fork of [Vix](https://github.com/get-vix/vix) — the same bubbletea/lipgloss/glamour TUI, with the
following Cortex-specific changes:

- **In-process session**: no `vixd` daemon. The session is a Go struct in `internal/session/`.
- **Cortex-aware provider**: talks to your Cortex backend (or OpenAI, Anthropic, Ollama — any OpenAI-compatible API).
- **Swarm orchestrator**: planner/developer/reviewer/tester/fixer roles, ported from the original Cortex CLI.
- **Tools**: read_file, write_file, edit_file, list_dir, search, run_shell.
- **Config**: lives at `~/.cortex/config.yaml` (matches the old TypeScript config schema).

## Build

```bash
# Add the vix-style Go toolchain to PATH
export PATH=$HOME/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.25.0.linux-amd64/bin:$PATH

# Build the binary
go build -o bin/cortex .

# Run
./bin/cortex chat          # interactive TUI
./bin/cortex ask "..."     # one-shot prompt
./bin/cortex goal "..."    # multi-agent swarm
./bin/cortex --list-models # show configured models
```

## Tests

```bash
go build -o bin/smoke ./cmd_smoke && ./bin/smoke
```

## Layout

```
main.go                              # CLI entry point
internal/
  config/      # vix's config (unchanged)
  protocol/    # event types copied from vix
  ui/          # vix's TUI (unchanged, ~10k lines)
  cortexconfig/  # NEW: user-facing config (YAML, ~/.cortex/config.yaml)
  provider/    # NEW: LLM client (OpenAI-compat + cortex-specific)
  tools/       # NEW: 6 built-in tools
  swarm/       # NEW: multi-agent orchestrator
  session/     # NEW: in-process chat session (replaces vixd)
  daemon/      # NEW: stub SessionClient (wraps session, exposes vix's API)
cmd_smoke/    # smoke test binary
```

## Why

Vix's UI is the best in class. The previous TypeScript port of Cortex CLI used
`neo-blessed` + `chalk`, which couldn't match the polish of `bubbletea` +
`lipgloss`. Forking the Go code directly gives us the exact vix visual fidelity
while letting us swap the LLM backend and tool set for Cortex.
