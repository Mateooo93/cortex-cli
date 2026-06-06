# AGENTS.md

This file provides guidance to AI coding agents when working on cortex-cli.

## Project Overview

cortex-cli is an interactive AI coding agent written in Go. It pairs
a Bubble Tea TUI (`internal/ui/`) with an in-process session
(`internal/session/`), a multi-provider LLM layer
(`internal/provider/`), a tool set (`internal/tools/`), and an
optional multi-agent swarm orchestrator (`internal/swarm/`).

The TUI client, daemon-stub, and protocol types are derived from
upstream vix but are not exposed to users — cortex-cli is shipped
as a single binary with no separate `vixd` daemon.

## Architecture

```
cmd/                    # (no entries; main.go is the single entry point)
internal/
  config/               # Config dir resolution (~/.cortex, ./.cortex)
  cortexconfig/         # User-facing YAML config + provider presets
  daemon/               # In-process SessionClient (stub over session.Session)
  provider/             # LLM provider adapters
    codex/              # ChatGPT OAuth + JWT + Bearer transport
  protocol/             # Shared types between client + daemon
  session/              # In-process chat session (replaces vixd)
  swarm/                # Multi-agent orchestrator (planner/developer/...)
  tools/                # Built-in tool set (read_file, write_file, bash, ...)
  ui/                   # Bubble Tea TUI components
```

`main.go` constructs a single `session.Session`, wraps it in the
`daemon.SessionClient` the UI expects, and launches the TUI.

## Development Commands

```bash
# Build the local cortex binary
make build

# Run all tests
make test

# Run a specific test
go test ./internal/ui/... -run TestRestoreChatHistoryVisibleAfterRestart -v

# Run the codex OAuth/JWT tests
go test ./internal/provider/codex/... -v

# Build a release tarball for the current platform
make release VERSION=v0.1.0
```

## Running

```bash
./bin/cortex                 # interactive TUI
./bin/cortex -p "fix the bug"   # one-shot prompt
./bin/cortex chat              # alias for the TUI
./bin/cortex ask "…"          # one-shot prompt (alternative)
./bin/cortex --list-models    # show configured providers/models
```

## Key Conventions

- **Go style** - follow standard Go conventions, use `gofmt`.
- **Error handling** - return errors, don't panic. Log with
  `log.Printf` in the session layer.
- **No over-engineering** - keep changes minimal and focused. Don't
  add abstractions for one-time operations.
- **Security** - sanitize all user inputs before shell execution.
  Be careful with tool execution paths. The `deny_list` in
  `~/.cortex/settings.json` is the single source of truth for
  what the agent may not touch.
- **Sync-friendly UI** - changes to `internal/ui/` should be
  minimal and port-friendly with upstream. Cortex-specific
  behaviour belongs in `internal/cortexconfig/`,
  `internal/provider/`, `internal/session/`, `internal/swarm/`,
  or `internal/tools/`.

## Environment

- **Go 1.26+** required
- One of: `CORTEX_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`,
  `CODEX_CODEX_TOKEN` (the ChatGPT-subscription JWT), or a local
  Ollama install. Set in the environment, in `.env`, or via the
  Settings tab (which stores in the OS keychain).
- The ChatGPT (codex) provider also accepts `CODEX_CODEX_TOKEN` for
  headless / CI use.

## Config directory resolution

By default cortex-cli merges config from two layered `.cortex`
directories: `~/.cortex` (user defaults) and `./.cortex`
(project overrides). This covers `settings.json`, `agents/`,
`skills/`, `AGENTS.md`, plus session state like `history.txt`,
`plans/`, `access_stats.db`, and `logs/`.

All path resolution flows through `config.CortexPaths`
(internal/config/paths.go). Add new `.cortex`-relative paths there
rather than hardcoding `filepath.Join(cwd, ".cortex", ...)`.

Pass `--config-dir /some/path` to use that directory as the sole
`.cortex` root. Neither `~/.cortex` nor `./.cortex` is consulted,
and all session state (history, plans, access stats, LLM logs) is
written inside the override directory. The directory is
auto-created and bootstrapped with default settings on first run.
This is useful for sandboxed / reproducible sessions without
touching real user or project config.

## Default access policy

The agent decides whether a path is accessible by default by
checking, in order: cwd, `$HOME`, the host's system directories
(per platform), or any entry in `allowed_directories`. Anything
outside that set surfaces as a confirmation prompt (interactive
sessions) or an error (headless). The `deny_list` always wins,
even if the path matches one of the auto-allow categories.

`$HOME` is auto-allowed in full (read + write). Lock down
sensitive subpaths via `deny_list.paths` (e.g. `~/.aws`,
`~/.ssh`, `~/.config/op`, `~/.kube`).

## Deny list

`settings.json` supports `deny_list` — paths and URLs that are
always off-limits. Use the structured form:

```json
"deny_list": {
  "paths": ["./secrets", "/etc/passwd"],
  "urls":  ["bad.example.com", "https://example.org/admin"]
}
```

The legacy flat-array form (`"deny_list": ["./secrets"]`) still
parses and is treated as paths-only. Deny takes precedence over
`allowed_directories`: a path that matches both is blocked. Path
entries may be absolute or relative to the config file that
declares them. Both lists are unioned across layered configs (home
+ project).

**Path match semantics**: a target path is blocked iff (after
symlink resolution and `Clean`) it equals a deny entry or is a
descendant of one.

**URL match semantics**:

- Entry with a scheme (e.g. `https://example.com/admin`) —
  URL-prefix match. Scheme and host are case-insensitive; path is
  case-sensitive and must align on `/`.
- Entry without a scheme (e.g. `example.com`) — hostname or
  dot-aligned suffix match (`api.example.com` matches
  `example.com`; `notexample.com` does not).

Coverage:

- `read_file` / `write_file` / `edit_file` / `delete_file` (and
  the minified variants): refused before execution when the
  target path is denied.
- `web_fetch`: refused when the `url` parameter matches a URL
  deny entry.
- `bash`: refused when any path-like token (a token that
  contains `/`) in the command resolves inside a denied path, or
  when any token containing `://` resolves to a denied URL. Bare
  words without `/` are not treated as paths, so prose like
  `echo 'no secrets here'` is allowed. Variable expansion,
  heredocs, and reassembly across variables are not analyzed
  (best-effort).
- `grep` / `glob_files`: matches inside a denied path are
  silently filtered from the output.
