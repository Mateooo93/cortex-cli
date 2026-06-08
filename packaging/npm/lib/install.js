"use strict";

const fs = require("fs");
const path = require("path");

const { downloadBinary } = require("./download");
const { warnIfShadowed } = require("./path-check");
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

  console.log(`cortex-cli: downloading ${asset} (${version})…`);
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
  // npm's default install output uses theme colors (often purple/magenta).
  process.env.NO_COLOR = process.env.NO_COLOR || "1";
  process.env.FORCE_COLOR = "0";
  try {
    const dest = await ensureBinary();
    if (dest) {
      console.log(`cortex-cli: installed native binary to ${dest}`);
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