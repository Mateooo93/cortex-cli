"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");

function packageRoot() {
  return path.join(__dirname, "..");
}

function readPackageVersion() {
  const pkgPath = path.join(packageRoot(), "package.json");
  const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf8"));
  if (!pkg.version || pkg.version === "0.0.0-dev") {
    throw new Error("cortex-cli npm package has no pinned release version");
  }
  return pkg.version;
}

function releaseRepo() {
  return process.env.CORTEX_CLI_REPO || "Mateooo93/cortex-cli";
}

function releaseBase() {
  return `https://github.com/${releaseRepo()}/releases`;
}

function cacheDir(version, asset) {
  return path.join(os.homedir(), ".cortex", "npm", version, asset);
}

module.exports = {
  packageRoot,
  readPackageVersion,
  releaseRepo,
  releaseBase,
  cacheDir,
};