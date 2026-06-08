"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");

const { normalizeVersion, versionsMatch } = require("../lib/version");

test("normalizeVersion strips v prefix", () => {
  assert.equal(normalizeVersion("v0.25.22"), "0.25.22");
  assert.equal(normalizeVersion("0.25.22"), "0.25.22");
});

test("versionsMatch compares normalized versions", () => {
  assert.equal(versionsMatch("v0.25.22", "0.25.22"), true);
  assert.equal(versionsMatch("0.25.20", "0.25.22"), false);
  assert.equal(versionsMatch(null, "0.25.22"), false);
});