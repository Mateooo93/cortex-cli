<p align="center">
  <img src="assets/Gemini_Generated_Image_d12qhnd12qhnd12q.png" alt="Cortex" width="400" />
</p>

<p align="center">
  <strong>Fast, token-efficient AI coding agent</strong> — single binary, polished terminal UI, 20+ providers.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=for-the-badge" alt="AGPL-3.0" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/stargazers"><img src="https://img.shields.io/github/stars/Mateooo93/cortex-cli?style=for-the-badge" alt="Stars" /></a>
</p>

## Install

### npm (macOS, Linux, Windows)

```bash
npm uninstall -g cortex-cli
npm install -g mateooo93-cortex
cortex
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.ps1 | iex
cortex
```

### Linux — amd64

```bash
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && sudo mv cortex /usr/local/bin/
cortex
```

### Linux — arm64

```bash
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-arm64
chmod +x cortex && sudo mv cortex /usr/local/bin/
cortex
```

### macOS — Apple Silicon

```bash
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-darwin-arm64
chmod +x cortex && sudo mv cortex /usr/local/bin/
cortex
```

### Build from source

```bash
git clone https://github.com/Mateooo93/cortex-cli.git
cd cortex-cli
go build -o cortex ./cmd/cortex
./cortex
```

One-shot prompt (no TUI): `cortex -p "your prompt"`

> **npm note:** The package `cortex-cli` on npm is a different product (CognitiveScale). Use `mateooo93-cortex`. If you get a 404, the package has not been published yet — use curl or the Windows PowerShell installer above.

## Features

### Terminal UI

- **Bubble Tea TUI** — markdown chat, live status bar, smooth animations, mouse support
- **Four tabs** — Sessions (F1), Chat (F2), Settings (F3), Workflows (F4)
- **Right info panel** (`Ctrl+B`) — context usage, model, keys, session stats
- **Model picker** (`/model`) — filterable overlay with auth badges per provider
- **Slash menu** — type `/` for commands; file path autocomplete while typing
- **Command palette** (`Ctrl+P`) — quick actions and tab switching
- **Input history** (`Ctrl+R`) — searchable past prompts
- **Image attachments** — paste or attach images in chat
- **Copy & export** — copy messages or the full conversation to clipboard

### Agent & tools

- **Single binary** — in-process session, no daemon, no IPC, instant startup
- **Parallel tool execution** — independent reads, greps, and probes run concurrently
- **Built-in tools** — `read_file`, `write_file`, `edit_file`, `delete_file`, `bash`, `grep`, `glob_files`, `web_search`, `web_fetch`
- **Precise edits** — multi-block exact edits with diff feedback (no huge patch dumps)
- **Sub-agents** — dispatch background agents that report back when done
- **Extended thinking** — rendered dim/italic; toggle visibility in the UI
- **Headless mode** — `cortex -p "prompt"` for scripts and CI

### Providers

- **20+ providers** — OpenAI, Anthropic, Gemini, Groq, Mistral, Ollama, vLLM, and more
- **Subscription OAuth** — ChatGPT (codex), Claude Pro/Max, GitHub Copilot via `/login`; tokens in OS keychain
- **Custom providers** — add any OpenAI-compatible gateway in Settings
- **Per-session model** — switch anytime with `/model` or `-m provider/model`

### Context & sessions

- **Persistent sessions** — chat history survives restarts; reopen from the Sessions tab
- **Multi-session** — run several chats (`Ctrl+T`); each keeps its own history and state
- **`/compact`** — summarize a long thread into a crisp summary while keeping decisions and file paths
- **Context warnings** — status bar shows token usage before you hit the limit
- **Layered config** — `~/.cortex` defaults + `./.cortex` project overrides

### Goals, workflows & ultracode

- **`/goal`** — set a measurable condition; the agent loops autonomously until an evaluator says it's met
- **`/workflow`** — spawn a multi-agent workflow (code, research, test, review, docs presets) alongside chat
- **Workflows tab (F4)** — live status, steps, and per-agent progress
- **`/effort`** — reasoning level from Low → Ultracode; ultracode auto-dispatches workflows on substantive tasks

### Safety

- **Deny list** — block sensitive paths and URLs in `settings.json` (always wins over allow rules)
- **Access prompts** — paths outside cwd/home/system dirs need confirmation before read/write
- **Shell sanitization** — bash commands checked against deny rules before execution

## Keyboard shortcuts

| Key | Action |
|-----|--------|
| `F1` | Sessions tab |
| `F2` | Chat tab |
| `F3` | Settings tab |
| `F4` | Workflows tab |
| `Ctrl+B` | Toggle right info panel |
| `Ctrl+P` | Command palette |
| `Ctrl+R` | Search input history |
| `Ctrl+T` | New session |
| `Enter` | Send message (interrupts current turn) |
| `Tab` | Queue message for after current turn |

## Slash commands

Type `/` in chat for the full menu.

| Command | Description |
|---------|-------------|
| `/model` | Pick provider + model |
| `/login` | OAuth sign-in (ChatGPT, Claude, Copilot) |
| `/goal` | Autonomous loop until a condition is met |
| `/workflow` | Start a multi-agent workflow with a prompt |
| `/effort` | Reasoning effort: low, medium, high, ultracode |
| `/compact` | Summarize history, keep decisions & paths |
| `/update` | Self-update to latest release (SHA-256 verified) |
| `/copy` | Copy full conversation to clipboard |
| `/clear` | Clear conversation history |

## CLI flags

```bash
cortex                          # interactive TUI
cortex -p "fix the bug"         # one-shot headless prompt
cortex -m openai/gpt-4o         # override model
cortex --workdir /path/to/proj  # set working directory
cortex --list-models            # list configured models
cortex --version                # print version
```

## Development

```bash
make build && make test
./bin/cortex          # interactive TUI
./bin/cortex -test    # TUI with fake data
```

## Credits

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, and Glamour. Forked from the [vix](https://github.com/get-vix/vix) agent/TUI design with cortex-specific providers, session, tools, and swarm layer.