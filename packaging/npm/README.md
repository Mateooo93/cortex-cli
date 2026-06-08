# mateooo93-cortex (npm)

Cross-platform npm wrapper for the [Mateooo93 cortex-cli](https://github.com/Mateooo93/cortex-cli) AI coding agent. On install it downloads the matching native binary from GitHub Releases.

> **Note:** The npm package `cortex-cli` is a different product (CognitiveScale). Use `mateooo93-cortex`.

```bash
npm uninstall -g cortex-cli
bun remove -g cortex-cli    # if you previously installed via bun
npm install -g mateooo93-cortex
hash -r                     # bash: refresh command cache
cortex
```

If `cortex` still shows CognitiveScale commands (`actions`, `agents`, …), another
install is ahead of npm on your `PATH`. Run `which -a cortex` — the first entry
must point at `.../mateooo93-cortex/shims/cortex.js`.

Set `CORTEX_SKIP_POSTINSTALL=1` to skip the binary download (for CI or offline mirrors).