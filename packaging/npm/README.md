# cortex-cli (npm)

Cross-platform npm wrapper for [cortex-cli](https://github.com/Mateooo93/cortex-cli). On install it downloads the matching native binary from GitHub Releases.

```bash
npm install -g cortex-cli
cortex
```

Also works with Bun:

```bash
bun install -g cortex-cli
cortex
```

Set `CORTEX_SKIP_POSTINSTALL=1` to skip the binary download (for CI or offline mirrors).