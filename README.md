<p align="center">
  <img src="assets/Gemini_Generated_Image_d12qhnd12qhnd12q.png" alt="Animated Cortex Logo" width="400" />
</p>

**Fast. Sleek. Token-efficient.**  
A beautiful single-binary AI coding agent with a polished terminal UI and 20+ providers out of the box (Cortex, OpenAI, ChatGPT subscriptions, Anthropic, Ollama, and more).

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=for-the-badge" alt="AGPL-3.0 License" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/stargazers"><img src="https://img.shields.io/github/stars/Mateooo93/cortex-cli?style=for-the-badge" alt="GitHub stars" /></a>
</p>

---

## Why you'll love it

- **Blazing fast** — true single binary with an in-process session. No separate daemon, no IPC, instant startup.
- **Stays in flow** — persistent chat history across restarts, smart `/compact` that summarizes while preserving decisions and file paths, automatic context warnings.
- **Beautiful, productive TUI** — clean Bubble Tea interface with live status bar, right-hand info panel, model picker, slash commands, and smooth animations.
- **20+ providers ready to go** — sign in with your existing ChatGPT, Claude Pro/Max, or GitHub Copilot subscription (no extra API keys). Plus OpenAI, Anthropic, Gemini, Groq, Ollama, vLLM, and easy custom OpenAI-compatible gateways.
- **Powerful built-in tools** — read/write/edit files (including fast multi-edit "pi-style" blocks), bash, grep, glob. Safe by default with deny-lists and confirmations.
- **Optional multi-agent swarm** — planner/developer/reviewer roles for big refactors when you need them.
- **Token-efficient by design** — extended thinking rendered dim/italic, auto-compact, precise edit tool that avoids sending huge diffs.

## Quick start

**Easiest (all platforms)** — npm or Bun downloads the native binary for your OS:

```bash
npm install -g cortex-cli
cortex
```

```bash
bun install -g cortex-cli
cortex
```

**Linux amd64** (see Install for other arches and package managers):

```bash
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && sudo mv cortex /usr/local/bin/
cortex
```

```bash
# One-shot prompt (no TUI)
cortex -p "explain the main agent loop in this repo"
```

On first run it creates `~/.cortex/config.yaml`. Use the Settings tab or `/model` to pick a provider and sign in.

## Core features

- **In-process everything** — fast, reliable, single binary.
- **Persistent sessions** — your history, todos, and context survive restarts. Switch between them with F1/F2.
- **/compact** — turn a 140k-token chat into a crisp 4k summary in seconds.
- **Rich edit tool** — safe, multi-block exact edits with diff + patch feedback.
- **Subscription auth** — ChatGPT (codex), Claude, Copilot — all via official OAuth, tokens in your OS keychain.
- **Swarm mode** — delegate planning, implementation, and review to specialized agents.
- **Extensible** — add skills, custom providers, or use the full tool surface from the TUI.

## Common commands

Type `/` in chat for the full menu. Highlights:

| Command   | What it does |
|-----------|--------------|
| `/model`  | Fast picker for any provider + model (with auth badges) |
| `/compact`| Summarize history while keeping decisions & paths |
| `/update` | Self-update to the latest release (verifies SHA-256) |
| `/login`  | OAuth sign-in for subscription providers |
| `Ctrl+B`  | Toggle the handy right info panel (context, keys, session stats) |

## Install

Pick one — they all install the same native binary.

### npm (macOS, Linux, Windows)

Requires [Node.js 18+](https://nodejs.org/). Downloads the correct release asset and verifies SHA-256.

```bash
npm install -g cortex-cli
cortex
```

### Bun (macOS, Linux, Windows)

```bash
bun install -g cortex-cli
cortex
```

### winget (Windows)

```powershell
winget install Mateooo93.Cortex
```

### Homebrew (macOS, Linux)

```bash
brew tap Mateooo93/cortex
brew install cortex
```

### Direct download

Binaries on the [Releases page](https://github.com/Mateooo93/cortex-cli/releases).

#### Linux

```bash
# amd64 (x86_64)
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && sudo mv cortex /usr/local/bin/

# arm64 (aarch64)
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-arm64
chmod +x cortex && sudo mv cortex /usr/local/bin/
```

#### macOS (Apple Silicon)

```bash
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-darwin-arm64
chmod +x cortex && sudo mv cortex /usr/local/bin/
```

#### Windows

Use **Windows Terminal** or **PowerShell** (`cmd.exe` has limited TUI support). Swap `amd64` for `arm64` on ARM PCs.

```powershell
mkdir "$env:LOCALAPPDATA\Programs\cortex" -Force; iwr "https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-windows-amd64.exe" -OutFile "$env:LOCALAPPDATA\Programs\cortex\cortex.exe"; & "$env:LOCALAPPDATA\Programs\cortex\cortex.exe"
```

Windows config lives at `%USERPROFILE%\.cortex\`. API keys and OAuth tokens use Windows Credential Manager.

### From source

Go 1.26+ on any platform:

```bash
git clone https://github.com/Mateooo93/cortex-cli.git
cd cortex-cli
go build -o cortex ./cmd/cortex
./cortex
```

On Windows, use `go build -o cortex.exe ./cmd/cortex` and run `.\cortex.exe`.

## Configuration (minimal)

Most people only need one of these:

```bash
# Subscription (no key)
# Just pick ChatGPT / Claude / Copilot in the UI

# OpenAI
export OPENAI_API_KEY=sk-...

# Anthropic
export ANTHROPIC_API_KEY=sk-ant-...

# Local
# Nothing to set — points at Ollama or your Cortex server by default
```

Full provider list and advanced YAML lives in `~/.cortex/config.yaml` or the Settings tab.

## Development

```bash
make build
make test
./bin/cortex -test   # TUI with fake data
```

## Credits & origin

Built on the excellent Bubble Tea + Lip Gloss + Glamour stack and the original vix TUI/agent design. Cortex-specific smarts (providers, session, swarm, tools) added on top.

Fast, focused, and pleasant to live in — that's the goal.

---

_This repo was transferred from my old account to this brand new one._