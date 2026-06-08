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
npm install -g @mateooo93/cortex-cli
cortex
```

### Bun (macOS, Linux, Windows)

```bash
bun install -g @mateooo93/cortex-cli
cortex
```

### Homebrew (macOS, Linux)

```bash
brew tap Mateooo93/cortex
brew install cortex
cortex
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.ps1 | iex
cortex
```

### winget (Windows)

```powershell
winget install Mateooo93.Cortex
cortex
```

> `winget install` works after [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) merges our manifest. Until then, use the PowerShell installer above.

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

> **npm note:** The unscoped package `cortex-cli` is a different product (CognitiveScale). If `cortex --help` shows pipelines or missions, uninstall it and use `@mateooo93/cortex-cli`.

## Features

- **Single binary** — in-process session, no daemon, instant startup
- **Persistent chat** — history survives restarts; switch sessions with F1/F2
- **20+ providers** — OpenAI, Anthropic, Gemini, Groq, Ollama, ChatGPT/Claude/Copilot subscriptions, custom gateways
- **Built-in tools** — read/write/edit files, bash, grep, glob; deny-lists and confirmations by default
- **Token-efficient** — `/compact` summarization, precise multi-block edits, dim thinking output
- **Workflows & swarm** — optional multi-agent orchestration for larger tasks

## Slash commands

Type `/` in chat for the full menu.

| Command | Description |
|---------|-------------|
| `/model` | Pick provider + model |
| `/compact` | Summarize history, keep decisions & paths |
| `/goal` | Autonomous loop until a condition is met |
| `/workflow` | Start a multi-agent workflow |
| `/effort` | Reasoning effort (low → ultracode) |
| `/login` | OAuth for subscription providers |
| `/update` | Self-update (SHA-256 verified) |
| `Ctrl+B` | Toggle right info panel |

## Development

```bash
make build && make test
./bin/cortex          # interactive TUI
./bin/cortex -test    # TUI with fake data
```

## Credits

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, and Glamour. Forked from the [vix](https://github.com/get-vix/vix) agent/TUI design with cortex-specific providers, session, tools, and swarm layer.