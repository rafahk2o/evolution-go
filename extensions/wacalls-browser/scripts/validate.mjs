import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const rootFlag = process.argv.indexOf("--root");
const root = path.resolve(rootFlag >= 0 ? process.argv[rootFlag + 1] : path.join(scriptDir, ".."));

function fail(message) {
  throw new Error(message);
}

function read(relative) {
  const target = path.join(root, relative);
  if (!fs.existsSync(target) || !fs.statSync(target).isFile()) fail(`Missing runtime file: ${relative}`);
  return fs.readFileSync(target, "utf8");
}

try {
  const manifest = JSON.parse(read("manifest.json"));
  if (manifest.manifest_version !== 3) fail("Manifest V3 is required.");
  if (manifest.content_scripts) fail("Content scripts are forbidden.");
  if (manifest.host_permissions) fail("Required host permissions are forbidden.");
  if (JSON.stringify(manifest.permissions || []) !== JSON.stringify(["storage"])) fail("Only storage may be a required permission.");
  if (!manifest.background || manifest.background.service_worker !== "service-worker.js") fail("Classic service worker is required.");
  const expectedIcons = ["16", "32", "48", "128"];
  for (const size of expectedIcons) {
    const manifestIcon = manifest.icons && manifest.icons[size];
    const actionIcon = manifest.action && manifest.action.default_icon && manifest.action.default_icon[size];
    if (!manifestIcon || manifestIcon !== actionIcon) fail(`Missing matching ${size}px extension icon.`);
    if (path.extname(manifestIcon).toLowerCase() !== ".png") fail(`Extension icon must be PNG: ${manifestIcon}`);
    const target = path.join(root, manifestIcon);
    if (!fs.existsSync(target) || !fs.statSync(target).isFile()) fail(`Missing runtime file: ${manifestIcon}`);
  }

  const runtime = new Set([
    "manifest.json",
    "service-worker.js",
    "call-window.html",
    "call-window.css",
    "call-window.js",
    "call-controller.js",
    "recording.js",
    "audio-worklet.js",
    "shared/core.js",
    "shared/protocol.js",
  ]);
  const html = read("call-window.html");
  for (const match of html.matchAll(/<(?:script|link)\b[^>]+(?:src|href)="([^"]+)"/gi)) {
    const reference = match[1];
    if (/^https?:\/\//i.test(reference)) fail(`Remote runtime resource is forbidden: ${reference}`);
    runtime.add(reference);
  }
  for (const match of html.matchAll(/<script(?:\s[^>]*)?>([\s\S]*?)<\/script>/gi)) {
    if (match[1].trim()) fail("Inline scripts are forbidden.");
  }

  for (const relative of runtime) {
    const content = read(relative);
    if (/\beval\s*\(|\bnew\s+Function\s*\(/.test(content)) fail(`Dynamic code execution is forbidden in ${relative}.`);
    if (relative !== "manifest.json" && /chatwoot/i.test(content)) fail(`Chatwoot coupling is forbidden in ${relative}.`);
  }
  console.log("Extension validation passed.");
} catch (error) {
  console.error(`Extension validation failed: ${error.message}`);
  process.exitCode = 1;
}
