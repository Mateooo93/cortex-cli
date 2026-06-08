"use strict";

const os = require("os");

/**
 * Map Node platform/arch to the cortex release asset basename.
 * Returns { asset, binaryName } where asset is the GitHub release filename.
 */
function resolveAsset() {
  const platform = os.platform();
  const arch = os.arch();

  if (platform === "darwin") {
    if (arch === "arm64") {
      return { asset: "cortex-darwin-arm64", binaryName: "cortex" };
    }
    throw unsupported(platform, arch, "macOS builds are Apple Silicon (arm64) only");
  }

  if (platform === "linux") {
    if (arch === "x64") {
      return { asset: "cortex-linux-amd64", binaryName: "cortex" };
    }
    if (arch === "arm64") {
      return { asset: "cortex-linux-arm64", binaryName: "cortex" };
    }
    throw unsupported(platform, arch);
  }

  if (platform === "win32") {
    if (arch === "x64") {
      return { asset: "cortex-windows-amd64.exe", binaryName: "cortex.exe" };
    }
    if (arch === "arm64") {
      return { asset: "cortex-windows-arm64.exe", binaryName: "cortex.exe" };
    }
    throw unsupported(platform, arch);
  }

  throw unsupported(platform, arch);
}

function unsupported(platform, arch, hint) {
  const msg =
    `cortex-cli: unsupported platform ${platform}/${arch}.` +
    (hint ? ` ${hint}` : "") +
    " Install a binary from https://github.com/Mateooo93/cortex-cli/releases";
  const err = new Error(msg);
  err.code = "UNSUPPORTED_PLATFORM";
  return err;
}

module.exports = { resolveAsset };