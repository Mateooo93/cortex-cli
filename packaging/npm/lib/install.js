"use strict";

const fs = require("fs");

const { downloadBinary, fetchText } = require("./download");
const { warnIfShadowed } = require("./path-check");
const { resolveAsset } = require("./platform");
const {
  cacheDir,
  currentBinaryPath,
  readPackageVersion,
  releaseBase,
  releaseRepo,
} = require("./paths");
const { updateCurrentSymlink } = require("./symlink");
const {
  isNewerVersion,
  normalizeVersion,
  readBinaryVersion,
  versionsMatch,
} = require("./version");

async function findNewerCurrentBinary(binaryName, pkgVersion) {
  const currentPath = currentBinaryPath(binaryName);
  if (!fs.existsSync(currentPath)) {
    return null;
  }
  const currentVersion = await readBinaryVersion(currentPath);
  if (isNewerVersion(currentVersion, pkgVersion)) {
    return currentPath;
  }
  return null;
}

async function fetchLatestReleaseVersion(fetchTextFn = fetchText) {
  const data = JSON.parse(
    await fetchTextFn(`https://api.github.com/repos/${releaseRepo()}/releases/latest`)
  );
  const version = normalizeVersion(data.tag_name);
  return version || null;
}

async function resolveInstallVersion(pkgVersion, fetchTextFn = fetchText) {
  if (process.env.CORTEX_NPM_PIN_PACKAGE === "1") {
    return pkgVersion;
  }
  try {
    const latestVersion = await fetchLatestReleaseVersion(fetchTextFn);
    if (isNewerVersion(latestVersion, pkgVersion)) {
      return latestVersion;
    }
  } catch {
    // GitHub Releases is the native-binary source of truth, but launch should
    // still work offline or when the API is temporarily unavailable.
  }
  return pkgVersion;
}

async function ensureBinary() {
  if (process.env.CORTEX_SKIP_POSTINSTALL === "1") {
    return null;
  }

  const pkgVersion = readPackageVersion();
  const { asset, binaryName } = resolveAsset();

  if (process.env.CORTEX_FORCE_REINSTALL !== "1") {
    const selfUpdatedPath = await findNewerCurrentBinary(binaryName, pkgVersion);
    if (selfUpdatedPath) {
      return selfUpdatedPath;
    }
  }

  const installVersion = await resolveInstallVersion(pkgVersion);
  const version = `v${installVersion}`;
  const destPath = cacheDir(installVersion, asset);

  let needsDownload =
    process.env.CORTEX_FORCE_REINSTALL === "1" || !fs.existsSync(destPath);

  if (!needsDownload) {
    const binaryVersion = await readBinaryVersion(destPath);
    if (!versionsMatch(binaryVersion, installVersion)) {
      console.warn(
        `cortex-cli: cached binary is ${binaryVersion || "unknown"}, expected ${installVersion}; re-downloading…`
      );
      needsDownload = true;
    }
  }

  if (needsDownload) {
    if (fs.existsSync(destPath)) {
      await fs.promises.unlink(destPath);
    }
    await downloadBinary({
      releaseBase: releaseBase(),
      version,
      asset,
      destPath,
    });
  }

  await updateCurrentSymlink(destPath, binaryName);

  return destPath;
}

async function main() {
  try {
    const dest = await ensureBinary();
    if (dest) {
      const binaryVersion = await readBinaryVersion(dest);
      console.log(
        `cortex-cli: installed native binary to ${dest}` +
          (binaryVersion ? ` (${binaryVersion})` : "")
      );
    }
    warnIfShadowed();
  } catch (err) {
    console.error(`cortex-cli: postinstall failed: ${err.message}`);
    process.exit(1);
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  ensureBinary,
  fetchLatestReleaseVersion,
  findNewerCurrentBinary,
  resolveInstallVersion,
};
