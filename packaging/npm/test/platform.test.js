"use strict";

const assert = require("assert");
const { resolveAsset } = require("../lib/platform");

// Smoke test: ensure mapping runs on the current host.
const { asset, binaryName } = resolveAsset();
assert.ok(asset.startsWith("cortex-"), `unexpected asset: ${asset}`);
assert.ok(binaryName === "cortex" || binaryName === "cortex.exe");
console.log(`platform ok: ${asset}`);