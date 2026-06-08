<p align="center">
  <img src="assets/Gemini_Generated_Image_d12qhnd12qhnd12q.png" alt="Cortex" width="400" />
</p>

<p align="center">
  <strong>A fast AI coding agent that lives in your terminal.</strong><br>
  One binary. Beautiful TUI. Your models, your machine.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=for-the-badge" alt="AGPL-3.0" /></a>
  <a href="https://github.com/Mateooo93/cortex-cli/stargazers"><img src="https://img.shields.io/github/stars/Mateooo93/cortex-cli?style=for-the-badge" alt="Stars" /></a>
</p>

## Highlights

- **Instant startup** — single binary, in-process session. No daemon, no Docker, no waiting.
- **Stays in flow** — persistent chat, multi-session support, `/compact` when context gets long.
- **Bring your own model** — OpenAI, Anthropic, Ollama, Groq, or sign in with ChatGPT / Claude / Copilot subscriptions.
- **Actually edits your code** — read, write, precise multi-block edits, bash, grep, web search. Safe by default.
- **Goes deep when you need it** — goals, multi-agent workflows, and ultracode mode for bigger tasks.

## Quick start

```bash
npm install -g mateooo93-cortex
cortex
```

Or download a binary (Linux amd64):

```bash
curl -L -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && sudo mv cortex /usr/local/bin/
cortex
```

One-shot, no TUI: `cortex -p "explain this repo"`

Pick a provider in the **Settings** tab or with `/model`. Subscription sign-in is `/login` — tokens stay in your OS keychain.

## What is cortex-cli?

cortex-cli is an interactive coding agent you run in the terminal. You describe what you want; it reads your repo, edits files, runs commands, and keeps working until the job is done — all inside a polished Bubble Tea interface.

It is built for daily use: sessions survive restarts, context usage is visible in the status bar, and you can queue messages or spin up parallel workflows without leaving chat. When a thread gets too long, `/compact` folds it into a short summary that keeps the decisions and file paths that matter.

Under the hood it is a fork of the [vix](https://github.com/get-vix/vix) agent design, extended with multi-provider support, workflows, goals, and a full built-in tool set.

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

> **npm note:** The package `cortex-cli` on npm is a different product (CognitiveScale). Use `mateooo93-cortex`. If you get a 404, the package has not been published yet — use curl or the Windows PowerShell installer above.

## Using it

**Tabs:** Sessions `F1` · Chat `F2` · Settings `F3` · Workflows `F4`

**Worth knowing:**
- `Ctrl+B` — right panel (context, model, keys)
- `Ctrl+P` — command palette
- `Ctrl+T` — new session
- `Enter` — send now · `Tab` — queue for after the current turn
- Type `/` — slash menu (`/model`, `/goal`, `/workflow`, `/compact`, `/effort`, `/update`, `/login`, `/copy`, `/clear`)

**CLI flags:**

```bash
cortex -p "fix the failing test"    # headless one-shot
cortex -m anthropic/claude-sonnet   # pick a model
cortex --workdir ./my-project       # set cwd
cortex --list-models                # show configured models
```

Config lives in `~/.cortex/` (Windows: `%USERPROFILE%\.cortex\`). Project overrides go in `./.cortex/`.

## Development

```bash
make build && make test
./bin/cortex
./bin/cortex -test    # TUI with fake data
```

## License

AGPL-3.0 — see [LICENSE](LICENSE).