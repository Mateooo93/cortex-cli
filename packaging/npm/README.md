# @mateooo93/cortex (npm)

npm wrapper that downloads the native `cortex` binary for your OS on `postinstall`.

> **Note:** The package `cortex-cli` on npmjs.org is a different product (CognitiveScale). Use `@mateooo93/cortex`.

## Install

**Recommended when npm auth fails:** use the install script (downloads the native binary, no registry token):

```bash
curl -fsSL https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.sh | bash
```

**npm** (GitHub Packages):

```bash
npm install -g @mateooo93/cortex@latest --registry=https://npm.pkg.github.com
```

Or set the scope once in `~/.npmrc`:

```
@mateooo93:registry=https://npm.pkg.github.com
```

Then:

```bash
npm install -g @mateooo93/cortex@latest
```

If you see `E401 Unauthorized`, the package may still be private — use the install script, or add a GitHub token with `read:packages`:

```
//npm.pkg.github.com/:_authToken=YOUR_GITHUB_TOKEN
```

The global `cortex` command must point at `.../node_modules/@mateooo93/cortex/shims/cortex.js` (or your package manager's global bin shim).

## Publish (maintainers)

**GitHub Packages** (default — uses `GITHUB_TOKEN` in Actions):

```bash
./script/publish-npm.sh v0.25.25 --yes
```

**npmjs.org** (optional legacy `mateooo93-cortex` — requires npm Automation token):

```bash
./script/publish-npm.sh v0.25.25 --yes --registry npmjs
```

After the first GitHub Packages publish, set the package visibility to **Public** under the repo's **Packages** tab so anyone can install without a token.