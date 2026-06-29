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

test("manager normalizes API keys before authentication", () => {
  const source = managerBundle();

  assert.match(source, /apiKey\.trim\(\)/);
  assert.match(source, /login:async\([^)]*\)=>\{[^}]*\.trim\(\)/);
});

test("manager login reads normalized interceptor status", () => {
  const source = managerBundle();
  const start = source.indexOf('console.error("Login error:",');

  assert.notEqual(start, -1, "login error handler not found");
  assert.match(source.slice(start, start + 400), /c==null\?void 0:c\.status/);
});

test("logout remains a client-only operation", () => {
  const source = managerBundle();
  const start = source.indexOf("logout:()=>");

  assert.notEqual(start, -1, "logout handler not found");
  const logout = source.slice(start, start + 350);
  assert.match(logout, /localStorage\.removeItem\("evolution-auth"\)/);
  assert.doesNotMatch(logout, /Xt\.(post|put|patch|delete)/);
});
