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

cortex-cli is a **remake of [vix](https://github.com/get-vix/vix)** —
the same Bubble Tea / Lip Gloss / Glamour TUI re-imagined as a
single-binary, multi-provider AI coding agent. The visual style,
keybindings, and agent loop stay faithful to vix; the Cortex-specific
changes are in the LLM layer, the session model, the tool set, and
the swarm orchestrator.

## Providers

cortex-cli ships with **20+ built-in provider presets** so a new
user can sign in without editing a config file. The list is grouped
by auth method:

### Subscriptions (OAuth sign-in, no API key needed)

If you already pay for one of these, **use this row** — you don't
need to buy API credits on top of your subscription.

| Provider | Default model | OAuth flow |
|----------|---------------|-----------|
| **ChatGPT (codex)** | `gpt-5.5` | Sign in with your ChatGPT Plus / Pro / Team / Enterprise account. The flow opens your browser, you approve on `auth.openai.com`, and the resulting token is stored in the OS keychain. [Source](https://github.com/openai/codex) |
| **Claude (Pro/Max)** | `claude-opus-4-8` | Sign in with your Claude Pro or Max plan. The token is captured via the Claude Code OAuth flow. |
| **GitHub Copilot** | `gpt-5.5` | Sign in with your GitHub account that has Copilot Pro / Pro+ / Max / Business. |

> **Important:** these providers do **not** use an API key. They
> authenticate with your existing subscription via the in-app
> browser flow. The `NeedsAPIKey` flag is `false` for all three.
> If the Settings tab shows you an API-key input for any of
> these, that's a bug — please open an issue.

### API-key providers (paid)

Get a key from the provider's dashboard, paste it into the
Settings tab (or set the env var). The key is stored in the OS
keychain.

| Provider | Default model | Env var | Get a key |
|----------|---------------|---------|-----------|
| **OpenAI** | `gpt-5.5` | `OPENAI_API_KEY` | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) |
| **Anthropic** | `claude-opus-4-8` | `ANTHROPIC_API_KEY` | [console.anthropic.com](https://console.anthropic.com/settings/keys) |
| **Google Gemini** | `gemini-2.5-pro` | `GEMINI_API_KEY` | [aistudio.google.com/apikey](https://aistudio.google.com/apikey) |
| **xAI (Grok)** | `grok-4` | `XAI_API_KEY` | [console.x.ai](https://console.x.ai) |
| **DeepSeek** | `deepseek-chat` | `DEEPSEEK_API_KEY` | [platform.deepseek.com](https://platform.deepseek.com/api_keys) |
| **Mistral AI** | `mistral-large-latest` | `MISTRAL_API_KEY` | [console.mistral.ai](https://console.mistral.ai/api-keys) |
| **Groq** | `llama-3.3-70b-versatile` | `GROQ_API_KEY` | [console.groq.com/keys](https://console.groq.com/keys) |
| **Cohere** | `command-r-plus` | `COHERE_API_KEY` | [dashboard.cohere.com](https://dashboard.cohere.com/api-keys) |
| **Perplexity** | `sonar-pro` | `PERPLEXITY_API_KEY` | [perplexity.ai/settings/api](https://www.perplexity.ai/settings/api) |

### Aggregators (one key, many models)

| Provider | Default model | Env var |
|----------|---------------|---------|
| **OpenRouter** | `anthropic/claude-opus-4-8` | `OPENROUTER_API_KEY` |
| **OpenGateway** | `minimax/minimax-m3` | `OPENGATEWAY_API_KEY` |
| **MiniMax** | `MiniMax-M2.7` | `MINIMAX_API_KEY` |
| **Xiaomi MiMo** | `mimo-v2.5-pro` | `MIMO_API_KEY` |
| **AWS Bedrock** | `anthropic.claude-opus-4-8` | `AWS_BEARER_TOKEN_BEDROCK` (OpenAI-compat Mantle endpoint) |

### Local / self-hosted (no key)

| Provider | Default model | Default URL |
|----------|---------------|-------------|
| **Cortex** | `cortex-code` | `http://127.0.0.1:8000/v1` |
| **Ollama** | `qwen3.5` | `http://127.0.0.1:11434/v1` |
| **LM Studio** | `qwen2.5-7b-instruct` | `http://127.0.0.1:1234/v1` |
| **vLLM** | `Llama-3.3-70B-Instruct` | `http://127.0.0.1:8001/v1` |

### Custom

If your provider isn't listed, use **Settings → Add custom
provider** and point it at any OpenAI-compatible gateway
(vLLM, LiteLLM, LM Studio, an internal inference server, etc.).
You can also edit `~/.cortex/config.yaml` directly.

## Features

- **In-process session** — single binary, no separate daemon.
- **Persistent sessions** — chat history survives across CLI
  restarts. The Sessions tab shows prior conversations.
- **Built-in tools** — `read_file`, `write_file`, `edit_file`,
  `bash`, `grep`, `glob_files`.
- **Multi-agent swarm** — optional planner / developer / reviewer
  / tester / fixer roles for larger refactors.
- **Status bar hints** — the bottom-left footer always shows the
  send / queue / cancel hint.
- **Extended thinking** — the model's hidden reasoning is
  rendered dim and italic so it never gets confused with the
  normal output.
- **ChatGPT OAuth** — sign in with your ChatGPT Plus / Pro / Team
  subscription; tokens stored in the OS keychain, transparently
  refreshed.

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

```powershell
# Windows (PowerShell) — amd64
Invoke-WebRequest -Uri https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-windows-amd64.exe -OutFile cortex.exe
Move-Item cortex.exe $env:LOCALAPPDATA\Microsoft\WindowsApps\cortex.exe
```

```powershell
# Windows ARM64 (Surface Pro X, Snapdragon X Elite, etc.)
Invoke-WebRequest -Uri https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-windows-arm64.exe -OutFile cortex.exe
Move-Item cortex.exe $env:LOCALAPPDATA\Microsoft\WindowsApps\cortex.exe
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
Plus / Pro / Team subscription via the official OpenAI OAuth flow
— same flow the OpenAI Codex CLI uses, same
`app_EMoamEEZ73f0CkXaXp7hrann` client_id, same
`auth.openai.com/oauth/authorize` endpoint. You do **not** need a
separate OpenAI API key.

1. Open the TUI and type `/model` in the chat input. A centered
   picker shows every model from every configured provider.
2. Pick `GPT-5.5 (ChatGPT)` and press **Enter** (or use the
   Settings tab → Providers → ChatGPT (codex) path).
3. **Your browser opens automatically** to `auth.openai.com`.
   The status bar shows *"Opening ChatGPT (codex) sign-in in your
   browser…"* while the local callback server comes up.
4. Sign in with your ChatGPT account and approve the device.
5. You're redirected back to `http://127.0.0.1:1455/auth/callback`.
   cortex-cli stores the OAuth token (access, refresh, JWT
   claims including `chatgpt-account-id`, `email`, `plan_type`,
   `exp`) in the OS keychain and switches the active model.

That's it — no intermediate *"press Enter to sign in"* panel,
no API-key prompt, no extra steps.

#### If the browser flow is blocked (SSH, WSL, or "Invalid authorize request")

If `auth.openai.com` redirects you to a phone-verification gate
(see [openai/codex#20161](https://github.com/openai/codex/issues/20161))
or you're on a remote machine with no browser on `localhost`,
use the device-code fallback:

1. Type `/login` in the TUI. A centered picker shows the three
   OAuth providers.
2. Type `codex --device` in the filter box. (The status line
   shows `codex — device-code`.)
3. Press **Enter**. The TUI prints a one-time code (e.g.
   `ABCD-1234`) and the verification URL
   `https://auth.openai.com/codex/device`.
4. Open that URL in **any browser on any device**, sign in to
   your ChatGPT account, and paste the code.
5. The TUI's status bar changes to *"Signed in to ChatGPT (codex)"*
   and the active model switches to your selection. The token
   is stored in the OS keychain just like the browser flow.

The device code is valid for 15 minutes; after that you'll need
to start over.

To sign out: in the Settings tab, open the **API Keys** manager
and press **Del** on the ChatGPT (codex) row.

The OAuth flow:

* **Callback URL:** `http://127.0.0.1:1455/auth/callback`
  (falls back to a random free port if 1455 is busy).
* **Authorize endpoint:** `https://auth.openai.com/oauth/authorize`
  with `client_id=app_EMoamEEZ73f0CkXaXp7hrann`, PKCE-S256,
  CSRF state, `originator=codex_cli_rs`.
* **Token endpoint:** `https://auth.openai.com/oauth/token`.
* **Device-code endpoints:** `/deviceauth/usercode` and
  `/deviceauth/token` (only used by the `--device` fallback).
* **Browser launch:** `xdg-open` (Linux) / `open` (macOS) /
  `wslview` (WSL). If none of these are available, the
  authorize URL is shown in the status bar — copy it into a
  browser manually.
* **JWT parsing:** reads the `https://api.openai.com/auth`
  custom claim to extract `chatgpt_account_id` and
  `chatgpt_plan_type`, plus the standard `email` and `exp`
  claims.
* **Keychain:** stores the JSON-encoded token under
  `service=cortex-cli`, `user=codex-oauth-token`. macOS
  Keychain / Linux Secret Service / Windows Credential Manager.
* **Refresh:** expired access tokens are refreshed
  transparently on next use; the new bundle replaces the old
  in the keychain.
* **CI / headless:** set `CODEX_CODEX_TOKEN=eyJ...` (a raw
  ChatGPT-subscription JWT) instead of going through the
  browser. The token is treated as already-signed-in.

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

## Slash commands

Type `/` in the chat input to open the slash menu. Available built-ins:

| Command | What it does |
|---------|--------------|
| `/model` | Opens a centered picker showing every model from every configured provider, with the provider name + auth method as a secondary line (e.g. `GPT-5.5 (ChatGPT)` with subtitle `codex · OAuth (subscription)`). Filter by typing. Enter selects. |
| `/compact` | Compresses the conversation history to free up context window space. The TUI asks the current model to summarize the conversation into a 5-10 bullet recap (preserving decisions, file paths, error messages, tool names), then replaces the history with `[summary, …last 4 messages]`. Status bar shows `compacted 142k → 4k tokens (97% reduction)`. If the LLM call fails, falls back to dropping the oldest half of the messages. |
| `/update` | Detects your OS + architecture, downloads the matching `cortex-<platform>` release from GitHub, verifies SHA-256 against `SHA256SUMS`, and replaces the current binary. Windows uses a detached helper process to handle the locked-file case (you can't delete a running `.exe`). User must restart after. |
| `/login` | Opens a centered picker for OAuth / subscription sign-in (codex, claude-sub, copilot). For codex, type `codex --device` in the filter box to use the device-code flow (no localhost browser needed) — the TUI shows a one-time code you enter at <https://auth.openai.com/codex/device> from any browser. |
| `/copy` | Copy the conversation to the clipboard. |
| `/clear` | Clear the current session's chat history. |
| `/skills` | List available skills. |

`/model` replaces the two-column model-selection picker that used
to live in the Settings tab. The Settings tab is now just:

- **Providers** — configure base URLs, API keys, OAuth sign-in.
  OAuth providers (codex / claude-sub / copilot) open a browser
  sign-in directly when you press Enter on them; no API-key form.
  Press **Tab** to move to Other Settings.
- **Other Settings** — theme, thinking display, reasoning effort,
  streaming, token-usage, **auto-compact context** (when enabled,
  automatically runs `/compact` once you exceed 80% of the
  model's context window). Press **Tab** to go back to Providers.

To switch models, use `/model` in the chat tab.

## Status bar, footer, and right panel

The bottom of the screen is a single slim line:

```
● connected   GPT-5.5 · codex   ctx 12k / 200k (6%)   ⏱ 2:13   1 queued   [F1] [F2] [F3]
```

- `● connected` (green), `● reconnecting` (yellow), or `● disconnected` (red).
- `GPT-5.5 · codex` is the active model + provider.
- `ctx 12k / 200k (6%)` is the context window usage. Counts use
  the model's reported `input_tokens` + `cache_read_tokens` with
  a chars/4 fallback so the bar always shows something. Turns
  yellow at 80% and red at 95%.
- `⏱ 2:13` is the elapsed time since the session started.
- `1 queued` appears when you have a pending message waiting
  (Tab queued a follow-up).
- `[F1] [F2] [F3]` are the tab switchers.

A transient message line appears **above** the slim footer when
something noteworthy happens (e.g. `⚠ context at 87% — auto-compacting…`,
`✖ ChatGPT sign-in failed`, `ℹ Refreshing models for openai`).

Press **Ctrl+B** to toggle the right-side info / status panel
(OpenCode-style). The panel is read-only and shows:

- **Model** — active model + provider
- **Context** — 20-cell progress bar with percentage + warning
  when auto-compact is about to fire
- **Session** — elapsed time, session count, queued count,
  connection state
- **Keys** — full keybind legend (F1/F2/F3, Tab, Enter, Esc,
  Ctrl+T, Ctrl+B, `/`)

The chat input keeps focus when the panel is open, so you can
keep typing while glancing at the stats.

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
defaultModel: codex/gpt-5.5

models:
  cortex:
    provider: cortex
    model: cortex-code
    baseUrl: http://127.0.0.1:8000/v1
  openai:
    provider: openai
    model: gpt-5.5
    baseUrl: https://api.openai.com/v1
    apiKey: sk-...
  codex:
    provider: codex
    model: gpt-5.5
    baseUrl: https://api.openai.com/v1
  anthropic:
    provider: anthropic
    model: claude-opus-4-8
    baseUrl: https://api.anthropic.com/v1
    apiKey: sk-ant-...
  ollama:
    provider: ollama
    model: qwen3.5
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
│   │   └── codex/           # ChatGPT OAuth + JWT + Bearer transport
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

## Origin

cortex-cli is a **remake of
[vix](https://github.com/get-vix/vix)**. The original vix copyright
is preserved in the [LICENSE](LICENSE) file; cortex-cli itself is
distributed under GNU AGPL-3.0.

## Credits

* The vix team for the original TUI, agent loop, and design
  philosophy: <https://github.com/get-vix/vix>
* The Bubble Tea, Lip Gloss, and Glamour teams at Charmbracelet
  for the TUI primitives.
* The cortex project for the provider gateway.

---

_This repo was transferred from my old account to this brand new one._
