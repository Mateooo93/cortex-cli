<p align="center">
  <img src="assets/cortex.svg" alt="Cortex" width="120" />
</p>

<p align="center">
  <strong>The open source AI coding agent for your terminal.</strong><br>
  One binary. Beautiful TUI. Your models, your machine.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square" alt="AGPL-3.0" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/stargazers"><img src="https://img.shields.io/github/stars/Mateooo93/cortex-cli?style=flat-square" alt="Stars" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/releases/latest"><img src="https://img.shields.io/github/v/release/Mateooo93/cortex-cli?style=flat-square" alt="Release" /></a>
</p>

<p align="center">
  <a href="https://github.com/Mateooo93/cortex-cli">
    <img src="assets/cortex-cli-screenshot.jpg" alt="cortex-cli — chat, tool calls, todos, and context panel" width="920" />
  </a>
</p>

<p align="center">
  <em>Chat, tool calls, todos, and the context panel in one terminal window.</em>
</p>

---

**cortex-cli** is a fast coding agent you run in the terminal. Describe what you want in plain language — it reads your repo, edits files, runs commands, searches the web, and keeps working until the job is done. Everything runs locally as a single Go binary with a polished Bubble Tea interface: no daemon, no Docker, no separate cloud agent.

## Table of contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Features](#features)
- [Authentication](#authentication)
- [Using it](#using-it)
- [Project memory](#project-memory)
- [Development](#development)
- [Contributing](#contributing)

## Installation

### curl (recommended)

macOS and Linux — no npm auth:

```bash
curl -fsSL https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.sh | bash
cortex --version
```

### npm

macOS, Linux, and Windows:

```bash
npm install -g @mateooo93/cortex@latest --registry=https://npm.pkg.github.com
cortex
```

If npm returns `E401 Unauthorized`, use the curl installer above, or add to `~/.npmrc`:

```
@mateooo93:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=YOUR_GITHUB_TOKEN
```

### Manual download

**Linux:**

```bash
curl -fsSL -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && mv cortex ~/.local/bin/
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.ps1 | iex
```

Other platforms and tarballs are on the [latest release](https://github.com/Mateooo93/cortex-cli/releases/latest). Build from source: `git clone`, `go build -o cortex ./cmd/cortex`, `./cortex`.

> **npm note:** Install `@mateooo93/cortex@latest` from GitHub Packages (command above). The package `cortex-cli` on npmjs.org is a different product. If `cortex` opens the wrong CLI, run `npm uninstall -g cortex-cli`, remove stale binaries like `~/.local/bin/cortex`, then `hash -r` and check `which -a cortex`.

## Quick start

```bash
cd your-project
cortex
```

One-shot without the TUI:

```bash
cortex -p "explain this repo"
cortex -m anthropic/claude-sonnet -p "fix the failing test"
cortex --list-models
```

## Features

- **Instant startup** — single binary, in-process session. No daemon, no Docker.
- **Stays in flow** — persistent chat, multi-session tabs, context usage in the status bar.
- **Bring your own model** — OpenAI, Anthropic, Ollama, Groq, OpenRouter, or sign in with ChatGPT / Claude / Copilot subscriptions.
- **Edits your code** — read, write, precise multi-block edits, bash, grep, web search. Safe by default with deny lists and path confirmation.
- **Goes deep** — goals, multi-agent workflows, ultracode effort, and `/compact` to fold long threads.
- **Remembers your project** — optional project-scoped memory under `.cortex/` (see [Project memory](#project-memory)).

## Authentication

**Subscription sign-in** — run `cortex`, open **Settings** (`F3`) or type `/login`, and sign in with ChatGPT (Codex), Claude, or Copilot. Tokens are stored in the OS keychain.

**API keys** — set `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `CORTEX_API_KEY`, or point at a local Ollama install. Keys can also be saved from Settings.

**Choose a model** — `/model` or the Settings tab. List configured options with `cortex --list-models`.

## Using it

**Tabs:** Sessions `F1` · Chat `F2` · Settings `F3` · Workflows `F4`

**Shortcuts:**

| Key | Action |
|-----|--------|
| `Ctrl+B` | Right panel (context, model, keys) |
| `Ctrl+P` | Command palette |
| `Ctrl+T` | New session |
| `Enter` | Send now |
| `Tab` | Queue for after the current turn |
| `/` | Slash menu |

**Slash commands:** `/model` · `/goal` · `/workflow` · `/effort` · `/compact` · `/update` · `/login` · `/memory` · `/copy` · `/clear`

Config lives in `~/.cortex/` (Windows: `%USERPROFILE%\.cortex\`). Project overrides go in `./.cortex/`. See [AGENTS.md](AGENTS.md) for architecture, deny-list semantics, and contributor conventions.

## Project memory

Cortex can persist durable project knowledge (preferences, conventions, architecture notes) under each repo's `.cortex/` directory. Browse and search with `/memory`, or let the agent save via the `memory_write` tool. Toggle in Settings.

Full details: [docs/memory.md](docs/memory.md).

## Development

```bash
make build && make test
./bin/cortex
./bin/cortex -test    # TUI with fake data
```

## Contributing

Bug reports, feature ideas, and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and conventions.

If cortex-cli helps you ship faster, a [star on GitHub](https://github.com/Mateooo93/cortex-cli) goes a long way.

## License

AGPL-3.0 — see [LICENSE](LICENSE).