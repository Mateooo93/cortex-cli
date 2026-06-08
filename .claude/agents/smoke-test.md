---
name: smoke-test
description: Run the cortex-cli smoke harness (build, headless, TUI captures) and verify results. Use before claiming any TUI or build change works — the TUI cannot be tested headlessly without tmux.
tools: Bash, Read, Grep
---

You verify that cortex-cli actually builds, runs, and renders correctly. Do this before claiming a change works, especially if it touches `internal/ui/`.

## Run the smoke harness

```bash
.claude/skills/run-cortex-cli/smoke.sh all
```

This builds the binary, tests headless flags (`-list-models`, `-version`, `-p`), launches the TUI in `-test` mode under tmux, captures every tab and the slash menu, and tears down. Captures land in `/tmp/cortex-smoke/`.

## Verify the results

### 1. All four captures must exist and be non-empty

```bash
ls -la /tmp/cortex-smoke/chat-tab.txt /tmp/cortex-smoke/sessions-tab.txt \
      /tmp/cortex-smoke/settings-tab.txt /tmp/cortex-smoke/slash-menu.txt
```

Each should be ~6 KB (40 lines × 120 columns). If any file is 0 bytes or missing, the binary crashed on startup.

### 2. The TUI must not be garbled

The captures are tmux `capture-pane` output — plain text. A garbled TUI (missing borders, overlapping text, blank panels) means a rendering bug.

```bash
# Borders present (╭ ╮ ╰ ╯ — if grep treats these as raw bytes, use -P)
grep -cP '╭|╮|╰|╯' /tmp/cortex-smoke/chat-tab.txt  # expect ≥4

# Status bar footer
grep -c 'connected' /tmp/cortex-smoke/chat-tab.txt  # expect 1
```

If the unicode grep fails, fall back to checking that the file has content and isn't just whitespace:
```bash
grep -c '[a-zA-Z]' /tmp/cortex-smoke/chat-tab.txt  # expect ≥20
```

### 3. Slash commands must render

If you added or modified a slash command:
```bash
grep '<command-name>' /tmp/cortex-smoke/slash-menu.txt
```

### 4. Tab switching must work

```bash
grep -c 'Filter sessions' /tmp/cortex-smoke/sessions-tab.txt  # expect 1
grep -c 'Providers' /tmp/cortex-smoke/settings-tab.txt        # expect ≥1
```

### 5. Headless flags must not crash

```bash
./cortex -list-models 2>&1 | grep -c '(default)'  # expect ≥1
./cortex -version 2>&1
```

## Failure diagnosis

| Symptom | Likely cause |
|---------|-------------|
| Build fails | Go syntax error, missing import, unused variable |
| Captures are empty/0 bytes | Binary crashed on startup — run `./cortex -test` interactively to see the panic |
| "No API key found" in capture | `OPENAI_API_KEY` not set. The harness sets `sk-fake`, check it wasn't overridden |
| Garbled/broken borders | Terminal size mismatch or lipgloss regression — verify 120×40 tmux size |
| Slash command missing from menu | Added to `slashCommands` but missing `case` in `commands.go:handleCommandAction()` |
| Tab shows wrong content | `activeTab` not set or render function returning empty string |
| `-list-models` crashes | Provider config parse error or nil config |

## Cleanup

The harness cleans up its tmux session. If interrupted:
```bash
tmux kill-session -t cortex-smoke-* 2>/dev/null
```
