"use strict";

const fs = require("fs");
const path = require("path");

const { downloadBinary } = require("./download");
const { resolveAsset } = require("./platform");
const { cacheDir, readPackageVersion, releaseBase } = require("./paths");

async function ensureBinary() {
  if (process.env.CORTEX_SKIP_POSTINSTALL === "1") {
    return null;
  }

  const version = `v${readPackageVersion()}`;
  const { asset, binaryName } = resolveAsset();
  const destPath = cacheDir(version.slice(1), asset);

  if (fs.existsSync(destPath)) {
    return destPath;
  }

  await downloadBinary({
    releaseBase: releaseBase(),
    version,
    asset,
    destPath,
  });

  // Convenience symlink: ~/.cortex/npm/current/<binaryName>
  const currentDir = path.join(path.dirname(path.dirname(destPath)), "current");
  await fs.promises.mkdir(currentDir, { recursive: true });
  const linkPath = path.join(currentDir, binaryName);
  try {
    await fs.promises.unlink(linkPath);
  } catch (err) {
    if (err.code !== "ENOENT") throw err;
  }
  try {
    await fs.promises.symlink(destPath, linkPath);
  } catch {
    // Windows may require elevated symlink rights; ignore.
  }

  return destPath;
}

async function main() {
  try {
    const dest = await ensureBinary();
    if (dest) {
      console.log(`cortex-cli: installed native binary to ${dest}`);
    }
  } catch (err) {
    console.error(`cortex-cli: postinstall failed: ${err.message}`);
    process.exit(1);
  }
}

if (require.main === module) {
  main();
}

module.exports = { ensureBinary };