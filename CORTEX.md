# Cortex CLI

A terminal-based AI coding agent. cortex-cli is a single Go binary
that pairs a Bubble Tea TUI with a multi-provider LLM backend, a
built-in tool set, an optional multi-agent swarm, and persistent
session storage.

## Features

- **In-process session** — no separate daemon. The session is a
  Go struct in `internal/session/`.
- **Multi-provider** — Cortex, OpenAI, ChatGPT (codex, via OAuth
  with your existing subscription), Anthropic, Ollama, OpenRouter,
  OpenGateway, MiniMax, Xiaomi MiMo, and any custom
  OpenAI-compatible gateway.
- **Swarm orchestrator** — planner / developer / reviewer / tester
  / fixer roles for larger refactors.
- **Tools** — `read_file`, `write_file`, `edit_file`, `list_dir`,
  `search`, `run_shell`.
- **Persistent sessions** — chat history survives across CLI
  restarts; `~/.cortex/` holds sessions, plans, history, and
  LLM logs.
- **Status-bar hints** — the bottom-left footer tells the user
  how to queue (`Tab`) or send + interrupt (`Enter`).

## Build

```bash
# Build the cortex binary
go build -o bin/cortex .

# Run
./bin/cortex chat          # interactive TUI (default)
./bin/cortex ask "..."     # one-shot prompt
./bin/cortex goal "..."    # multi-agent swarm
./bin/cortex --list-models # show configured models
```

## Tests

```bash
go test ./...                                  # run everything
go test ./internal/provider/codex/... -v      # OAuth + JWT
go test ./internal/ui/... -v                   # TUI behaviour
```

## Layout

```
main.go                          # CLI entry point
internal/
  config/                        # Config dir resolution (~/.cortex)
  protocol/                      # Event types
  ui/                            # Bubble Tea TUI
  cortexconfig/                  # User-facing config (YAML, ~/.cortex/config.yaml)
  provider/                      # LLM clients (OpenAI-compat + cortex-specific)
    codex/                       # ChatGPT OAuth + JWT
  tools/                         # Built-in tool set
  swarm/                         # Multi-agent orchestrator
  session/                       # In-process chat session
  daemon/                        # SessionClient stub wrapping the session
```

## Why a single binary

cortex-cli is shipped as a single Go binary with no separate
daemon. The TUI starts a chat session in-process, talks to the
configured LLM provider over the network, and writes session state
to `~/.cortex/`. This keeps the install trivial (one binary, one
config file) and avoids the Unix-socket coordination the original
vix design needed.
