"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");

const { findNewerCurrentBinary, resolveInstallVersion } = require("../lib/install");
const { currentBinaryPath } = require("../lib/paths");

function withHome(t) {
  const oldHome = process.env.HOME;
  const home = fs.mkdtempSync(path.join(os.tmpdir(), "cortex-npm-test-"));
  process.env.HOME = home;
  t.after(() => {
    if (oldHome === undefined) {
      delete process.env.HOME;
    } else {
      process.env.HOME = oldHome;
    }
    fs.rmSync(home, { recursive: true, force: true });
  });
  return home;
}

function writeFakeBinary(filePath, version) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `#!/bin/sh\necho cortex ${version}\n`);
  fs.chmodSync(filePath, 0o755);
}

test("findNewerCurrentBinary returns self-updated current binary", async (t) => {
  if (process.platform === "win32") {
    t.skip("shell-script fake binary is POSIX-only");
    return;
  }

  const home = withHome(t);
  const target = path.join(home, ".cortex", "npm", "0.25.23", "cortex-linux-amd64");
  writeFakeBinary(target, "0.25.23");

  const current = currentBinaryPath("cortex");
  fs.mkdirSync(path.dirname(current), { recursive: true });
  fs.symlinkSync(target, current);

  assert.equal(await findNewerCurrentBinary("cortex", "0.25.22"), current);
});

test("findNewerCurrentBinary ignores equal or older current binary", async (t) => {
  if (process.platform === "win32") {
    t.skip("shell-script fake binary is POSIX-only");
    return;
  }

  const home = withHome(t);
  const target = path.join(home, ".cortex", "npm", "0.25.22", "cortex-linux-amd64");
  writeFakeBinary(target, "0.25.22");

  const current = currentBinaryPath("cortex");
  fs.mkdirSync(path.dirname(current), { recursive: true });
  fs.symlinkSync(target, current);

  assert.equal(await findNewerCurrentBinary("cortex", "0.25.22"), null);
  assert.equal(await findNewerCurrentBinary("cortex", "0.25.23"), null);
});

test("resolveInstallVersion prefers newer GitHub release over package version", async () => {
  const got = await resolveInstallVersion("0.25.39", async () =>
    JSON.stringify({ tag_name: "v0.25.42" })
  );
  assert.equal(got, "0.25.42");
});

test("resolveInstallVersion falls back to package version when GitHub lookup fails", async () => {
  const got = await resolveInstallVersion("0.25.39", async () => {
    throw new Error("network down");
  });
  assert.equal(got, "0.25.39");
});

test("resolveInstallVersion can be pinned to package version", async (t) => {
  const oldPin = process.env.CORTEX_NPM_PIN_PACKAGE;
  process.env.CORTEX_NPM_PIN_PACKAGE = "1";
  t.after(() => {
    if (oldPin === undefined) {
      delete process.env.CORTEX_NPM_PIN_PACKAGE;
    } else {
      process.env.CORTEX_NPM_PIN_PACKAGE = oldPin;
    }
  });

  const got = await resolveInstallVersion("0.25.39", async () =>
    JSON.stringify({ tag_name: "v0.25.42" })
  );
  assert.equal(got, "0.25.39");
});
