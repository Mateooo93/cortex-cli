# mateooo93-cortex (npm)

Cross-platform npm wrapper for the [Mateooo93 cortex-cli](https://github.com/Mateooo93/cortex-cli) AI coding agent. On install it downloads the matching native binary from GitHub Releases.

> **Note:** The npm package `cortex-cli` is a different product (CognitiveScale). Use `mateooo93-cortex`.

```bash
npm uninstall -g cortex-cli
npm install -g mateooo93-cortex
cortex
```

Set `CORTEX_SKIP_POSTINSTALL=1` to skip the binary download (for CI or offline mirrors).