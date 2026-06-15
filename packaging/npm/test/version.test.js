"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const {
  compareVersions,
  isNewerVersion,
  normalizeVersion,
  versionsMatch,
} = require("../lib/version");

test("normalizeVersion strips v prefix", () => {
  assert.equal(normalizeVersion("v0.25.22"), "0.25.22");
  assert.equal(normalizeVersion("0.25.22"), "0.25.22");
});

test("versionsMatch compares normalized versions", () => {
  assert.equal(versionsMatch("v0.25.22", "0.25.22"), true);
  assert.equal(versionsMatch("0.25.20", "0.25.22"), false);
  assert.equal(versionsMatch(null, "0.25.22"), false);
});

test("compareVersions orders release versions", () => {
  assert.equal(compareVersions("0.25.23", "0.25.22"), 1);
  assert.equal(compareVersions("0.25.22", "0.25.23"), -1);
  assert.equal(compareVersions("v0.25.22", "0.25.22"), 0);
});

test("compareVersions treats stable releases as newer than prereleases", () => {
  assert.equal(compareVersions("0.25.23", "0.25.23-rc.1"), 1);
  assert.equal(compareVersions("0.25.23-rc.2", "0.25.23-rc.1"), 1);
});

test("isNewerVersion only accepts strict upgrades", () => {
  assert.equal(isNewerVersion("0.25.23", "0.25.22"), true);
  assert.equal(isNewerVersion("0.25.22", "0.25.22"), false);
  assert.equal(isNewerVersion(null, "0.25.22"), false);
});
