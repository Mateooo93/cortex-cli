# @mateooo93/cortex-cli (npm)

Cross-platform npm wrapper for the [Mateooo93 cortex-cli](https://github.com/Mateooo93/cortex-cli) AI coding agent. On install it downloads the matching native binary from GitHub Releases.

> **Note:** The unscoped npm package `cortex-cli` is a different product (CognitiveScale Cortex). Use the scoped package below.

```bash
npm uninstall -g cortex-cli   # remove CognitiveScale package if present
npm install -g @mateooo93/cortex-cli
cortex
```

Also works with Bun:

```bash
bun install -g @mateooo93/cortex-cli
cortex
```

Set `CORTEX_SKIP_POSTINSTALL=1` to skip the binary download (for CI or offline mirrors).