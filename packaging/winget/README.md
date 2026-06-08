# winget packaging

Manifests for `Mateooo93.Cortex`. Updated on each release by `script/publish-winget.sh`.

## User install

```powershell
winget install Mateooo93.Cortex
```

This only works after the package is merged into [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs).

## Maintainer setup (one time)

1. Fork https://github.com/microsoft/winget-pkgs
2. Create a GitHub PAT with `public_repo` scope
3. Add repo secret **`WINGET_TOKEN`** on `Mateooo93/cortex-cli`
4. Run **Actions → Submit winget manifest** (or wait for the release workflow)

The action opens a PR to your `winget-pkgs` fork; merge it upstream when CI passes.

## Local manifest test

```powershell
winget install --manifest .\packaging\winget\Mateooo93.Cortex
```