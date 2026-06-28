"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const root = path.join(__dirname, "..");
const validator = path.join(root, "scripts", "validate.mjs");

function validate(target) {
  return spawnSync(process.execPath, [validator, "--root", target], { encoding: "utf8" });
}

test("validates the real unpacked extension tree", () => {
  const result = validate(root);
  assert.equal(result.status, 0, result.stderr || result.stdout);
  assert.match(result.stdout, /Extension validation passed/);
});

test("rejects remote runtime scripts", () => {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "wacalls-extension-"));
  fs.cpSync(root, temporary, { recursive: true, filter(source) { return !source.includes(path.sep + "dist") && !source.includes(path.sep + "artifacts"); } });
  fs.appendFileSync(path.join(temporary, "call-window.html"), '\n<script src="https://example.com/x.js"></script>\n');
  const result = validate(temporary);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr + result.stdout, /remote/i);
  fs.rmSync(temporary, { recursive: true, force: true });
});

test("manifest remains independent from pages and required hosts", () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(root, "manifest.json"), "utf8"));
  assert.equal(manifest.content_scripts, undefined);
  assert.equal(manifest.host_permissions, undefined);
  assert.equal(JSON.stringify(manifest).toLowerCase().includes("chatwoot"), false);
});

test("packager includes the recording runtime", () => {
  const script = fs.readFileSync(path.join(root, "scripts", "package.ps1"), "utf8");
  assert.match(script, /"recording\.js"/);
});

test("validator rejects a runtime tree without the recorder", () => {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "wacalls-extension-"));
  try {
    fs.cpSync(root, temporary, { recursive: true, filter(source) { return !source.includes(path.sep + "dist") && !source.includes(path.sep + "artifacts"); } });
    fs.rmSync(path.join(temporary, "recording.js"));
    const result = validate(temporary);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr + result.stdout, /recording\.js/i);
  } finally {
    fs.rmSync(temporary, { recursive: true, force: true });
  }
});
