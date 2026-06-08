# @mateooo93/cortex (npm)

npm wrapper that downloads the native `cortex` binary for your OS on `postinstall`.

> **Note:** The package `cortex-cli` on npmjs.org is a different product (CognitiveScale). Use `@mateooo93/cortex`.

## Install

```bash
npm install -g @mateooo93/cortex --registry=https://npm.pkg.github.com
```

Or set the scope once in `~/.npmrc`:

```
@mateooo93:registry=https://npm.pkg.github.com
```

Then:

```bash
npm install -g @mateooo93/cortex
```

The global `cortex` command must point at `.../node_modules/@mateooo93/cortex/shims/cortex.js` (or your package manager's global bin shim).

## Publish (maintainers)

**GitHub Packages** (default — uses `GITHUB_TOKEN` in Actions):

```bash
./script/publish-npm.sh v0.25.19 --yes
```

**npmjs.org** (optional legacy `mateooo93-cortex` — requires npm Automation token):

```bash
./script/publish-npm.sh v0.25.19 --yes --registry npmjs
```

After the first GitHub Packages publish, set the package visibility to **Public** under the repo's **Packages** tab so anyone can install without a token.