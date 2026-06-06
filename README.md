<div align="center">


# cortex-cli

A sleek, fast, token-efficient AI coding agent. Multi-provider
(Cortex, OpenAI, Anthropic, Ollama) with a polished terminal UI.

<p align="center">
  <a href="https://github.com/Mateooo93/cortex-cli/releases"><img src="https://img.shields.io/github/v/release/Mateooo93/cortex-cli?style=for-the-badge&color=green" alt="Latest release" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=for-the-badge" alt="AGPL-3.0 License" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/stargazers"><img src="https://img.shields.io/github/stars/Mateooo93/cortex-cli?style=for-the-badge" alt="GitHub stars" /></a>
</p>
</div>

---

## What is cortex-cli?

cortex-cli is a fork of **[vix](https://github.com/get-vix/vix)**
with cortex-specific changes:

* **In-process session** (no separate `vixd` daemon) — simpler
  to run, easier to embed, single binary.
* **Cortex-aware provider layer** — first-class support for the
  Cortex gateway, plus OpenAI, Anthropic, and Ollama.
* **Persistent sessions** — chat history survives across CLI
  restarts, the Sessions tab shows prior conversations, and
  the bottom-left footer tells you how to queue a follow-up
  message (`Tab`) or send + interrupt (`Enter`).
* **Smarter tool defaults** — `run_shell` uses `bash` (with `sh`
  fallback) so one-liners work the same on macOS and Linux.
* **Cleaner thinking display** — extended thinking is rendered
  dim and italic so it never gets confused with the assistant's
  normal output.

The visual style, keybindings, and overall UX are intentionally
identical to vix. cortex-cli is a thin layer of cortex-aware
plumbing on top of the same bubbletea + lipgloss + glamour
frontend.

## Install

### Pre-built binary

Grab the latest release for your platform from
[GitHub Releases](https://github.com/Mateooo93/cortex-cli/releases):

```bash
# macOS arm64
curl -L -o cortex \
  https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-darwin-arm64
chmod +x cortex && sudo mv cortex /usr/local/bin/
```

```bash
# Linux amd64
curl -L -o cortex \
  https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && sudo mv cortex /usr/local/bin/
```

### From source

Requires Go 1.26+.

```bash
git clone https://github.com/Mateooo93/cortex-cli.git
cd cortex-cli
go build -o cortex .
./cortex --version
```

## Quick start

```bash
# Set your Cortex API key (or any other provider's key)
export CORTEX_API_KEY=sk-...

# Launch the TUI
cortex

# Or run a single prompt non-interactively
cortex -p "summarise the README of the current repo"
```

On first launch cortex-cli will:

1. Create `~/.cortex/config.yaml` with default providers.
2. Prompt for an API key if none is configured.
3. Open the chat tab with the active model.

## Keybindings

| Key                | Action                                    |
|--------------------|-------------------------------------------|
| `Enter`            | Send (interrupts after current edit)      |
| `Tab`              | Queue (run after this turn)               |
| `Esc`              | Cancel the current operation              |
| `F1`               | Sessions tab                              |
| `F2`               | Chat tab                                  |
| `F3`               | Settings tab                              |
| `Ctrl+T`           | New session                               |
| `Ctrl+N` / `Ctrl+P`| Next / previous session                   |
| `Ctrl+R`           | Search input history                      |
| `Shift+Enter`      | Newline (in input editor)                 |
| `Ctrl+C`           | Quit (confirms before exit)               |

The bottom-left status bar always shows the send / queue / cancel
hint when no other status message is active.

## Configuration

`~/.cortex/config.yaml`:

```yaml
defaultModel: cortex

models:
  cortex:
    provider: cortex
    model: cortex
    baseUrl: https://api.cortex.ai/v1
    apiKey: sk-...
  openai:
    provider: openai
    model: gpt-4o
    baseUrl: https://api.openai.com/v1
    apiKey: sk-...
  anthropic:
    provider: anthropic
    model: claude-3-5-sonnet-20241022
    baseUrl: https://api.anthropic.com/v1
    apiKey: sk-ant-...
  ollama:
    provider: ollama
    model: llama3.2
    baseUrl: http://localhost:11434/v1
```

You can also set keys via environment variables; see
`cortex --list-models` for the names your build supports.

## Project layout

```
.
├── main.go                  # CLI entry point
├── internal/
│   ├── config/              # vix-compatible config (used by UI)
│   ├── cortexconfig/        # Cortex YAML config + provider presets
│   ├── daemon/              # In-process SessionClient (was vixd)
│   ├── provider/            # LLM provider adapters
│   ├── protocol/            # Shared types between client + daemon
│   ├── session/             # In-process session implementation
│   ├── swarm/               # Optional swarm execution
│   ├── tools/               # Built-in tool set (bash, edit, search...)
│   └── ui/                  # Bubble Tea TUI (statusbar, sessions, etc.)
└── Makefile
```

## Development

```bash
# Build everything
make build

# Run the test suite
make test

# Run a single test verbosely
go test ./internal/ui/... -run TestRestoreChatHistoryVisibleAfterRestart -v

# Test the TUI with fake data
./bin/cortex -test
```

## License

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE).

cortex-cli is a fork of [vix](https://github.com/get-vix/vix).
The original vix copyright is preserved in the LICENSE file.

## Credits

* The vix team for the original TUI, agent loop, and design
  philosophy: <https://github.com/get-vix/vix>
* The Bubble Tea, Lip Gloss, and Glamour teams at Charmbracelet
  for the TUI primitives.
* The cortex project for the provider gateway.

---

_This repo was transferred from my old account to this brand new one._
