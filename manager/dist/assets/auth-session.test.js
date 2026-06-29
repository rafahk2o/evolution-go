"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

function managerBundle() {
  const bundle = fs
    .readdirSync(__dirname)
    .find((name) => /^index-.*\.js$/.test(name));
  assert.ok(bundle, "manager JavaScript bundle not found");
  return fs.readFileSync(path.join(__dirname, bundle), "utf8");
}

test("an unrelated 401 does not erase persisted global credentials", () => {
  const source = managerBundle();

  assert.match(source, /Unauthorized request/);
  assert.doesNotMatch(source, /Unauthorized - clearing auth data/);
});
