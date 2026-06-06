<div align="center">


# cortex-cli

A sleek, fast, token-efficient AI coding agent. Multi-provider
(Cortex, OpenAI, ChatGPT, Anthropic, Ollama) with a polished
terminal UI.

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=for-the-badge" alt="AGPL-3.0 License" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/stargazers"><img src="https://img.shields.io/github/stars/Mateooo93/cortex-cli?style=for-the-badge" alt="GitHub stars" /></a>
</p>
</div>

---

## What is cortex-cli?

cortex-cli is an interactive terminal-based AI coding agent with
session persistence, tool execution, and a multi-provider LLM
backend. The same binary can talk to:

* **Cortex** — the project's home gateway (`http://127.0.0.1:8000/v1`)
* **OpenAI** — direct API access with an `OPENAI_API_KEY`
* **ChatGPT (codex)** — your existing ChatGPT subscription, signed
  in via OAuth (no separate API key required)
* **Anthropic** — Claude models with an `ANTHROPIC_API_KEY`
* **Ollama** — local models on `http://127.0.0.1:11434/v1`
* **OpenRouter, OpenGateway, MiniMax, Xiaomi MiMo** — preset
  multi-model gateways

Highlights:

* **In-process session** — single binary, no separate daemon.
* **Persistent sessions** — chat history survives across CLI
  restarts. The Sessions tab shows prior conversations.
* **Built-in tools** — `read_file`, `write_file`, `edit_file`,
  `bash`, `grep`, `glob_files`.
* **Multi-agent swarm** — optional planner / developer / reviewer
  / tester / fixer roles for larger refactors.
* **Status bar hints** — the bottom-left footer always shows the
  send / queue / cancel hint.
* **Extended thinking** — the model's hidden reasoning is
  rendered dim and italic so it never gets confused with the
  normal output.

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

```bash
# Linux arm64
curl -L -o cortex \
  https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-arm64
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
# Launch the TUI
cortex

# Or run a single prompt non-interactively
cortex -p "summarise the README of the current repo"
```

On first launch cortex-cli creates `~/.cortex/config.yaml` with
default providers. The Settings tab walks you through picking a
provider and authenticating.

## Provider setup

cortex-cli ships with provider presets; you only need to
authenticate the one you want to use.

### OpenAI (paid API key)

```bash
export OPENAI_API_KEY=sk-...
```

…or paste the key into the Settings tab.

### ChatGPT (codex) — use your existing subscription

The **ChatGPT (codex)** provider authenticates with your ChatGPT
Plus / Pro / Team subscription, so you don't need a separate
OpenAI API key.

1. Open the TUI and switch to the **Settings** tab (F3).
2. In the left column, pick **ChatGPT (codex)**.
3. In the right column, pick a model (e.g. `GPT-5 (ChatGPT)`)
   and press **Enter**.
4. The "Sign in with ChatGPT" prompt opens. Press **Enter** —
   your browser opens to `auth.openai.com`.
5. Approve the device. cortex-cli stores the resulting OAuth
   token in your OS keychain and switches the active model.

To sign out: in the Settings tab, open the **API Keys** manager
and press **Del** on the ChatGPT (codex) row.

The OAuth flow:

* Listens on `http://127.0.0.1:1455/auth/callback` (falls back
  to a random free port if 1455 is busy).
* Opens the default browser via `xdg-open` (Linux) / `open`
  (macOS) / `wslview` (WSL). If none of these are available,
  the authorize URL is shown in the status bar — copy it into
  a browser manually.
* Reads the JWT and stores the access token, refresh token,
  email, plan type, and `chatgpt-account-id` claim in the
  keychain. Expired access tokens are refreshed transparently.
* CI / headless: set `CODEX_CODEX_TOKEN=eyJ...` instead of going
  through the browser.

### Anthropic (Claude)

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

…or paste the key into the Settings tab.

### Ollama (local)

Nothing to configure. cortex-cli points at
`http://127.0.0.1:11434/v1` by default and uses a dummy bearer
key.

### Custom provider

Settings → **Add custom provider**. Any OpenAI-compatible
gateway (vLLM, LiteLLM, LM Studio, …) works — just point at its
`/v1` base URL.

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
defaultModel: codex/gpt-5

models:
  cortex:
    provider: cortex
    model: cortex-code
    baseUrl: http://127.0.0.1:8000/v1
  openai:
    provider: openai
    model: gpt-5
    baseUrl: https://api.openai.com/v1
    apiKey: sk-...
  codex:
    provider: codex
    model: gpt-5
    baseUrl: https://api.openai.com/v1
  anthropic:
    provider: anthropic
    model: claude-sonnet-4-5
    baseUrl: https://api.anthropic.com/v1
    apiKey: sk-ant-...
  ollama:
    provider: ollama
    model: llama3.2
    baseUrl: http://localhost:11434/v1
```

API keys can also come from the environment (see
`cortex --list-models`) or from the OS keychain.

## Project layout

```
.
├── main.go                  # CLI entry point
├── internal/
│   ├── config/              # Config dir resolution (.cortex)
│   ├── cortexconfig/        # User-facing YAML config + provider presets
│   ├── daemon/              # In-process SessionClient
│   ├── provider/            # LLM provider adapters
│   │   └── codex/           # ChatGPT OAuth + JWT + responses client
│   ├── protocol/            # Shared types between client + daemon
│   ├── session/             # In-process session implementation
│   ├── swarm/               # Optional multi-agent orchestrator
│   ├── tools/               # Built-in tool set (bash, edit, search...)
│   └── ui/                  # Bubble Tea TUI (statusbar, sessions, etc.)
└── Makefile
```

## Development

```bash
# Build the local cortex binary
make build

# Run the test suite
make test

# Run a single test verbosely
go test ./internal/ui/... -run TestRestoreChatHistoryVisibleAfterRestart -v

# Run only the codex OAuth/JWT tests
go test ./internal/provider/codex/... -v

# Test the TUI with fake data
./bin/cortex -test
```

## License

GNU Affero General Public License v3.0 — see [LICENSE](LICENSE).

## Credits

* The Bubble Tea, Lip Gloss, and Glamour teams at Charmbracelet
  for the TUI primitives.
* The cortex project for the provider gateway.

---

_This repo was transferred from my old account to this brand new one._
