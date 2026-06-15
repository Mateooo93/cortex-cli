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

function splitVersion(v) {
  const normalized = normalizeVersion(v).split("+", 1)[0];
  const [core, prerelease = ""] = normalized.split("-", 2);
  const nums = core.split(".").map((part) => {
    const n = Number.parseInt(part, 10);
    return Number.isFinite(n) ? n : 0;
  });
  while (nums.length < 3) nums.push(0);
  return { nums, prerelease };
}

function comparePrerelease(a, b) {
  if (a === b) return 0;
  if (!a) return 1;
  if (!b) return -1;

  const left = a.split(".");
  const right = b.split(".");
  const len = Math.max(left.length, right.length);
  for (let i = 0; i < len; i += 1) {
    if (left[i] === undefined) return -1;
    if (right[i] === undefined) return 1;
    if (left[i] === right[i]) continue;

    const ln = /^\d+$/.test(left[i]) ? Number.parseInt(left[i], 10) : null;
    const rn = /^\d+$/.test(right[i]) ? Number.parseInt(right[i], 10) : null;
    if (ln !== null && rn !== null) return Math.sign(ln - rn);
    if (ln !== null) return -1;
    if (rn !== null) return 1;
    return left[i] < right[i] ? -1 : 1;
  }
  return 0;
}

function compareVersions(a, b) {
  const left = splitVersion(a);
  const right = splitVersion(b);
  for (let i = 0; i < Math.max(left.nums.length, right.nums.length); i += 1) {
    const diff = (left.nums[i] || 0) - (right.nums[i] || 0);
    if (diff !== 0) return Math.sign(diff);
  }
  return comparePrerelease(left.prerelease, right.prerelease);
}

function isNewerVersion(candidateVersion, baseVersion) {
  if (!candidateVersion || !baseVersion) return false;
  return compareVersions(candidateVersion, baseVersion) > 0;
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

module.exports = {
  normalizeVersion,
  versionsMatch,
  compareVersions,
  isNewerVersion,
  readBinaryVersion,
};
