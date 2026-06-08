"use strict";

const fs = require("fs");
const path = require("path");

const { downloadBinary } = require("./download");
const { warnIfShadowed } = require("./path-check");
const { resolveAsset } = require("./platform");
const { cacheDir, readPackageVersion, releaseBase } = require("./paths");
const { updateCurrentSymlink } = require("./symlink");
const { readBinaryVersion, versionsMatch } = require("./version");

async function ensureBinary() {
  if (process.env.CORTEX_SKIP_POSTINSTALL === "1") {
    return null;
  }

  const pkgVersion = readPackageVersion();
  const version = `v${pkgVersion}`;
  const { asset, binaryName } = resolveAsset();
  const destPath = cacheDir(pkgVersion, asset);

  let needsDownload =
    process.env.CORTEX_FORCE_REINSTALL === "1" || !fs.existsSync(destPath);

  if (!needsDownload) {
    const binaryVersion = await readBinaryVersion(destPath);
    if (!versionsMatch(binaryVersion, pkgVersion)) {
      console.warn(
        `cortex-cli: cached binary is ${binaryVersion || "unknown"}, package requires ${pkgVersion}; re-downloading…`
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

module.exports = { ensureBinary };