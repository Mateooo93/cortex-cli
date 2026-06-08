"use strict";

const { execFile } = require("child_process");
const { promisify } = require("util");

const execFileAsync = promisify(execFile);

function normalizeVersion(v) {
  if (!v) return "";
  return String(v).trim().replace(/^v/i, "");
}

function versionsMatch(binaryVersion, packageVersion) {
  return normalizeVersion(binaryVersion) === normalizeVersion(packageVersion);
}

async function readBinaryVersion(binaryPath) {
  try {
    const { stdout } = await execFileAsync(binaryPath, ["--version"], {
      timeout: 15_000,
      windowsHide: true,
    });
    const line = String(stdout).trim().split(/\r?\n/)[0] || "";
    const match = line.match(/\bv?(\d+\.\d+\.\d+(?:[-+][\w.-]+)?)\b/i);
    return match ? match[1] : null;
  } catch {
    return null;
  }
}

module.exports = { normalizeVersion, versionsMatch, readBinaryVersion };