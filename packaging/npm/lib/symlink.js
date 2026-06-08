"use strict";

const fs = require("fs");
const path = require("path");

async function updateCurrentSymlink(destPath, binaryName) {
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
}

module.exports = { updateCurrentSymlink };