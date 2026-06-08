"use strict";

const fs = require("fs");
const path = require("path");
const { execSync } = require("child_process");

const CONFLICT_MARKERS = [
  "cortex-cli/bin/cortex",
  "node_modules/cortex-cli/",
  "CognitiveScale",
  "cortex actions",
];

function npmGlobalBin() {
  try {
    const prefix = execSync("npm prefix -g", {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    return path.join(prefix, "bin", "cortex");
  } catch {
    return null;
  }
}

function pathEntries() {
  return (process.env.PATH || "")
    .split(path.delimiter)
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function resolveCortexOnPath() {
  for (const dir of pathEntries()) {
    const candidate = path.join(dir, process.platform === "win32" ? "cortex.cmd" : "cortex");
    if (!fs.existsSync(candidate)) {
      continue;
    }
    try {
      return fs.realpathSync(candidate);
    } catch {
      return candidate;
    }
  }
  return null;
}

function isOurInstall(target) {
  if (!target) {
    return false;
  }
  const normalized = target.replace(/\\/g, "/");
  if (normalized.endsWith("/shims/cortex.js")) {
    return true;
  }
  if (normalized.includes("/mateooo93-cortex/") || normalized.includes("/@mateooo93/cortex/")) {
    return true;
  }
  if (normalized.includes("/.cortex/npm/")) {
    return true;
  }
  return false;
}

function looksLikeCognitiveScale(target) {
  if (!target) {
    return false;
  }
  const normalized = target.replace(/\\/g, "/");
  if (CONFLICT_MARKERS.some((marker) => normalized.includes(marker))) {
    return true;
  }
  if (isOurInstall(target)) {
    return false;
  }
  // Heuristic: CognitiveScale ships a cortex.js launcher, not our shim layout.
  if (normalized.endsWith("/cortex.js")) {
    return true;
  }
  return false;
}

function warnIfShadowed() {
  const shimPath = npmGlobalBin();
  const firstOnPath = resolveCortexOnPath();

  if (!shimPath || !firstOnPath) {
    return;
  }

  let same = false;
  try {
    same = fs.realpathSync(shimPath) === fs.realpathSync(firstOnPath);
  } catch {
    same = shimPath === firstOnPath;
  }

  if (same || isOurInstall(firstOnPath)) {
    return;
  }

  console.warn("");
  console.warn("cortex-cli: another `cortex` command comes first on your PATH:");
  console.warn(`  ${firstOnPath}`);
  console.warn(`@mateooo93/cortex shim: ${shimPath}`);
  console.warn("");
  if (looksLikeCognitiveScale(firstOnPath)) {
    console.warn("Remove the conflicting CognitiveScale CLI, then open a new terminal:");
    console.warn("  npm uninstall -g cortex-cli");
    console.warn("  bun remove -g cortex-cli   # if installed via bun");
  } else {
    console.warn("Remove or move the other binary (often ~/.local/bin/cortex from a prior /update),");
    console.warn("ensure the npm global bin directory is before ~/.local/bin on PATH, then open a new shell.");
  }
  console.warn("");
  console.warn("Or run this package directly until PATH is fixed:");
  console.warn(`  ${shimPath}`);
  console.warn("");
}

module.exports = { warnIfShadowed, resolveCortexOnPath, looksLikeCognitiveScale };