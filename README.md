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
— same flow the OpenAI Codex CLI uses, same `app_oauth_agent`
client_id, same `auth.openai.com/oauth/authorize` endpoint.
You do **not** need a separate OpenAI API key.

1. Open the TUI and switch to the **Settings** tab (F3).
2. In the left column, pick **ChatGPT (codex)**.
3. In the right column, pick a model (e.g. `GPT-5.5 (ChatGPT)`)
   and press **Enter**.
4. **Your browser opens automatically** to `auth.openai.com`.
   The status bar shows *"Opening ChatGPT sign-in in your
   browser…"* while the local callback server comes up.
5. Sign in with your ChatGPT account and approve the device.
6. You're redirected back to `http://127.0.0.1:1455/auth/callback`.
   cortex-cli stores the OAuth token (access, refresh, JWT
   claims including `chatgpt-account-id`, `email`, `plan_type`,
   `exp`) in the OS keychain and switches the active model.

That's it — no intermediate *"press Enter to sign in"* panel,
no API-key prompt, no extra steps. The single Enter on the
codex model row is what kicks off the browser.

To sign out: in the Settings tab, open the **API Keys** manager
and press **Del** on the ChatGPT (codex) row.

The OAuth flow:

* **Callback URL:** `http://127.0.0.1:1455/auth/callback`
  (falls back to a random free port if 1455 is busy).
* **Authorize endpoint:** `https://auth.openai.com/oauth/authorize`
  with `client_id=app_oauth_agent`, PKCE-S256, CSRF state.
* **Token endpoint:** `https://auth.openai.com/oauth/token`.
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
