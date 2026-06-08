---
name: run-cortex-cli
description: Build, launch, smoke-test, and screenshot the cortex-cli TUI and headless modes. Use when asked to "run cortex", "build cortex", "test the TUI", "screenshot cortex", or verify a TUI change renders correctly.
---

# run-cortex-cli

Single-binary Go TUI AI coding agent. The driver is `smoke.sh` at the skill root. Paths are relative to the repo root (where `go.mod` lives).

## Prerequisites

```bash
sudo apt-get install -y tmux golang-go
```

Go 1.26+ required.

## Build

```bash
go build -o cortex ./cmd/cortex/
```

## Run (agent path)

```bash
# Full smoke: build + headless + TUI captures
.claude/skills/run-cortex-cli/smoke.sh all

# Headless only (no tmux needed)
.claude/skills/run-cortex-cli/smoke.sh headless

# TUI only (requires tmux)
.claude/skills/run-cortex-cli/smoke.sh tui
```

Captures land in `/tmp/cortex-smoke/` as `.txt` files (tmux `capture-pane` output — 120×40 terminal renders).

## Run (human path)

```bash
OPENAI_API_KEY=sk-... ./cortex          # interactive TUI
OPENAI_API_KEY=sk-fake ./cortex -test   # TUI with fake data, no API key
./cortex -p "explain this file"         # headless one-shot
./cortex -list-models                   # show configured providers
```

## Gotchas

- **API key required to launch TUI.** Without any key, the app blocks at "No API key found." Set a fake key to bypass for UI testing.
- **`-test` mode** fills chat with fake data but still needs an API key to bypass the initial prompt.
- **Go 1.26+ is a hard requirement.** Older Go fails with syntax errors.
- **Headless `-p` reads stdin if no argument follows.** Always pipe or provide a prompt string.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Build fails | Go < 1.26. `go version`. |
| "No API key found" blocks TUI | `OPENAI_API_KEY=sk-fake ./cortex -test` |
| TUI renders garbled | `TERM=xterm-256color` not set |
| `-p` hangs | Missing prompt argument |
| Tmux session lingers | `tmux kill-session -t cortex-smoke-*` |
