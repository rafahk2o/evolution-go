"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const root = path.join(__dirname, "..");

test("declares a minimal standalone MV3 extension", () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(root, "manifest.json"), "utf8"));
  assert.equal(manifest.manifest_version, 3);
  assert.deepEqual(manifest.background, { service_worker: "service-worker.js" });
  assert.equal(manifest.action.default_popup, undefined);
  assert.deepEqual(manifest.permissions, ["storage"]);
  assert.ok(manifest.optional_host_permissions.includes("https://*/*"));
  assert.ok(manifest.optional_host_permissions.includes("http://localhost/*"));
  assert.ok(manifest.optional_host_permissions.includes("http://127.0.0.1/*"));
  assert.equal(manifest.host_permissions, undefined);
  assert.equal(manifest.content_scripts, undefined);
  assert.equal(JSON.stringify(manifest).includes("<all_urls>"), false);
});

test("references local runtime files", () => {
  const manifest = JSON.parse(fs.readFileSync(path.join(root, "manifest.json"), "utf8"));
  assert.equal(fs.existsSync(path.join(root, manifest.background.service_worker)), true);
  assert.equal(fs.existsSync(path.join(root, "call-window.html")), true);
});
