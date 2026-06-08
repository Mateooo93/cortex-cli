"use strict";

const crypto = require("crypto");
const fs = require("fs");
const https = require("https");
const path = require("path");

function get(url) {
  return new Promise((resolve, reject) => {
    const req = https.get(url, { headers: { "User-Agent": "cortex-cli-npm" } }, (res) => {
      if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        get(res.headers.location).then(resolve, reject);
        return;
      }
      if (res.statusCode !== 200) {
        reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        res.resume();
        return;
      }
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => resolve(Buffer.concat(chunks)));
      res.on("error", reject);
    });
    req.on("error", reject);
  });
}

async function fetchText(url) {
  const buf = await get(url);
  return buf.toString("utf8");
}

async function sha256OfFile(filePath) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash("sha256");
    const stream = fs.createReadStream(filePath);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", () => resolve(hash.digest("hex")));
    stream.on("error", reject);
  });
}

function parseSha256Sums(text, assetName) {
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const match = trimmed.match(/^([a-f0-9]{64})\s{2}(.+)$/i);
    if (!match) continue;
    if (match[2] === assetName) {
      return match[1].toLowerCase();
    }
  }
  return null;
}

async function downloadBinary({ releaseBase, version, asset, destPath }) {
  const sumsURL = `${releaseBase}/download/${version}/SHA256SUMS`;
  const assetURL = `${releaseBase}/download/${version}/${asset}`;

  const sumsText = await fetchText(sumsURL);
  const expected = parseSha256Sums(sumsText, asset);
  if (!expected) {
    throw new Error(`SHA256SUMS missing entry for ${asset} (${sumsURL})`);
  }

  await fs.promises.mkdir(path.dirname(destPath), { recursive: true });
  const tmpPath = `${destPath}.download`;
  const data = await get(assetURL);
  await fs.promises.writeFile(tmpPath, data);

  const got = await sha256OfFile(tmpPath);
  if (got !== expected) {
    await fs.promises.unlink(tmpPath).catch(() => {});
    throw new Error(`checksum mismatch for ${asset}: expected ${expected}, got ${got}`);
  }

  await fs.promises.rename(tmpPath, destPath);
  if (process.platform !== "win32") {
    await fs.promises.chmod(destPath, 0o755);
  }
}

module.exports = { downloadBinary, parseSha256Sums };