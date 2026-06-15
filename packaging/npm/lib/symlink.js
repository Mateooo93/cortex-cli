"use strict";

const fs = require("fs");
const path = require("path");

const { currentBinaryPath } = require("./paths");

async function updateCurrentSymlink(destPath, binaryName) {
  const linkPath = currentBinaryPath(binaryName);
  const currentDir = path.dirname(linkPath);
  await fs.promises.mkdir(currentDir, { recursive: true });
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
}

module.exports = { updateCurrentSymlink };
