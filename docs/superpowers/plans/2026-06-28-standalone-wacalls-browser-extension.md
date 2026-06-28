# Standalone WACalls Browser Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Chrome/Edge Manifest V3 extension that stores one Evolution GO URL/API key and opens a compact window for outbound WhatsApp voice calls with bidirectional audio, mute, status, duration, and hang-up.

**Architecture:** A classic Manifest V3 service worker stores the credential and exposes only typed, allowlisted Evolution requests. A dedicated extension popup window owns the UI and all live browser media resources; shared pure modules validate input and normalize API responses.

**Tech Stack:** Manifest V3, vanilla JavaScript, Chrome Extensions APIs, WebRTC, AudioWorklet, Node.js built-in test runner, PowerShell packaging.

---

## File Structure

All runtime and test files live under `extensions/wacalls-browser/`:

- `manifest.json`: MV3 metadata, minimal permissions, background worker, and toolbar action.
- `service-worker.js`: singleton window management, trusted configuration storage, permission checks, and allowlisted Evolution API calls.
- `shared/core.js`: pure URL, phone, status, call, and error normalization.
- `shared/protocol.js`: internal message types and strict payload validation.
- `call-window.html`: accessible configuration, dialer, and active-call markup.
- `call-window.css`: compact floating-window presentation and state styles.
- `call-window.js`: DOM bindings, exact-origin permission request, rendering, and lifecycle hooks.
- `call-controller.js`: dependency-injected call state machine, microphone, WebRTC, polling, mute, hang-up, and cleanup.
- `audio-worklet.js`: 16 kHz mono PCM capture and playback processors.
- `tests/*.test.js`: manifest, pure helper, worker, controller, window, and worklet tests.
- `scripts/validate.mjs`: static manifest and runtime-package validation.
- `scripts/package.ps1`: safe `dist/` rebuild and versioned ZIP creation.
- `package.json`: dependency-free test, validation, and package commands.
- `README.md`: installation, configuration, security boundary, and troubleshooting.

No content script, Chatwoot file, backend route, or Go package changes.

### Task 1: Scaffold and validate the Manifest V3 extension

**Files:**
- Create: `extensions/wacalls-browser/package.json`
- Create: `extensions/wacalls-browser/manifest.json`
- Create: `extensions/wacalls-browser/service-worker.js`
- Create: `extensions/wacalls-browser/call-window.html`
- Create: `extensions/wacalls-browser/call-window.css`
- Create: `extensions/wacalls-browser/call-window.js`
- Create: `extensions/wacalls-browser/tests/manifest.test.js`

- [ ] **Step 1: Write the failing manifest test**

Create `tests/manifest.test.js` with assertions for Manifest V3, a classic
service worker, toolbar action without `default_popup`, `storage`, the optional
HTTPS and loopback origins, and absence of content scripts or `<all_urls>`:

```js
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
```

- [ ] **Step 2: Run RED**

Run:

```powershell
node --test extensions/wacalls-browser/tests/manifest.test.js
```

Expected: FAIL with `ENOENT` for `manifest.json`.

- [ ] **Step 3: Add the minimal manifest and package scripts**

Create `manifest.json`:

```json
{
  "manifest_version": 3,
  "name": "Evolution GO WACalls",
  "version": "0.1.0",
  "description": "Chamadas de voz WhatsApp pela Evolution GO.",
  "permissions": ["storage"],
  "optional_host_permissions": [
    "https://*/*",
    "http://localhost/*",
    "http://127.0.0.1/*"
  ],
  "background": { "service_worker": "service-worker.js" },
  "action": { "default_title": "Abrir Evolution GO WACalls" }
}
```

Create `package.json`:

```json
{
  "name": "evolution-go-wacalls-browser",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "test": "node --test tests/*.test.js",
    "validate": "node scripts/validate.mjs",
    "package": "powershell -ExecutionPolicy Bypass -File scripts/package.ps1"
  }
}
```

Create a minimal `service-worker.js` that opens `call-window.html` from
`chrome.action.onClicked`, a valid HTML shell loading `call-window.css` and
`call-window.js`, and an empty strict-mode `call-window.js`. Do not add remote
scripts or inline JavaScript.

- [ ] **Step 4: Run GREEN**

Run:

```powershell
node --test extensions/wacalls-browser/tests/manifest.test.js
```

Expected: 2 tests pass.

- [ ] **Step 5: Commit the scaffold**

```powershell
git add extensions/wacalls-browser
git commit -m "feat: scaffold standalone WACalls extension"
```

### Task 2: Implement pure validation and normalization helpers

**Files:**
- Create: `extensions/wacalls-browser/shared/core.js`
- Create: `extensions/wacalls-browser/tests/core.test.js`

- [ ] **Step 1: Write failing helper tests**

Create `tests/core.test.js` that requires `shared/core.js` and covers the public
API below:

```js
test("accepts HTTPS origins and loopback HTTP only", () => {
  assert.equal(core.normalizeApiOrigin("https://api.example.com/"), "https://api.example.com");
  assert.equal(core.normalizeApiOrigin("http://localhost:8080"), "http://localhost:8080");
  assert.throws(() => core.normalizeApiOrigin("http://api.example.com"), /HTTPS/);
  assert.throws(() => core.normalizeApiOrigin("https://user:pass@api.example.com"), /credenciais/);
  assert.throws(() => core.normalizeApiOrigin("https://api.example.com/v1"), /origem/);
});

test("normalizes and validates international phone numbers", () => {
  assert.equal(core.normalizePhone("+55 (11) 99999-9999"), "5511999999999");
  assert.throws(() => core.normalizePhone("123"), /DDI e DDD/);
  assert.throws(() => core.normalizePhone("1".repeat(16)), /DDI e DDD/);
});

test("normalizes the real instance status response", () => {
  assert.deepEqual(core.normalizeInstanceStatus({ data: { Connected: true, LoggedIn: true, Name: "Suporte" } }), {
    connected: true,
    loggedIn: true,
    instanceName: "Suporte"
  });
});

test("normalizes calls and terminal states", () => {
  assert.deepEqual(core.normalizeCall({ callId: "c1", status: "connected", peer: "5511@s.whatsapp.net" }), {
    callId: "c1", direction: "", status: "connected", peer: "5511"
  });
  assert.equal(core.isTerminalStatus("ended"), true);
  assert.equal(core.isTerminalStatus("failed"), true);
  assert.equal(core.isTerminalStatus("connected"), false);
});
```

Also test Portuguese error mapping for 401, 409, 422, 503, 504, network
failure, and safe use of an upstream `error` string no longer than 240 chars.

- [ ] **Step 2: Run RED**

Run:

```powershell
node --test extensions/wacalls-browser/tests/core.test.js
```

Expected: FAIL because `shared/core.js` does not exist.

- [ ] **Step 3: Implement the UMD-style pure module**

Expose `WaCallsCore` in browsers and `module.exports` in Node with exactly:

```js
{
  normalizeApiOrigin,
  permissionPattern,
  normalizePhone,
  normalizeInstanceStatus,
  normalizeCall,
  isTerminalStatus,
  callErrorMessage
}
```

Use `new URL`, reject username/password/query/hash/non-root paths, accept HTTP
only for `localhost` and `127.0.0.1`, and return `${origin}/*` from
`permissionPattern`. Phone numbers must contain 8 through 15 digits. Treat
`ended`, `rejected`, and `failed` as terminal. Strip the JID suffix from peers.

Implement error selection in this order: bounded upstream `error`, bounded
upstream `message`, mapped HTTP status, then
`"Falha inesperada ao controlar a chamada."`.

- [ ] **Step 4: Run GREEN**

Run:

```powershell
node --test extensions/wacalls-browser/tests/core.test.js
```

Expected: all helper tests pass.

- [ ] **Step 5: Commit helpers**

```powershell
git add extensions/wacalls-browser/shared extensions/wacalls-browser/tests/core.test.js
git commit -m "feat: validate standalone WACalls inputs"
```

### Task 3: Implement typed messages, configuration, and singleton window management

**Files:**
- Create: `extensions/wacalls-browser/shared/protocol.js`
- Modify: `extensions/wacalls-browser/service-worker.js`
- Modify: `extensions/wacalls-browser/call-window.html`
- Create: `extensions/wacalls-browser/tests/protocol.test.js`
- Create: `extensions/wacalls-browser/tests/service-worker.test.js`

- [ ] **Step 1: Write failing protocol tests**

Define and test these only message names:

```js
const TYPES = {
  CONFIG_GET: "CONFIG_GET",
  CONFIG_SAVE: "CONFIG_SAVE",
  CALL_START: "CALL_START",
  CALL_WEBRTC: "CALL_WEBRTC",
  CALL_ACTIVE: "CALL_ACTIVE",
  CALL_END: "CALL_END"
};
```

Assert `validateMessage` rejects unknown keys, caller-supplied `url`, `method`,
or `headers`, API keys outside `CONFIG_SAVE`, phone numbers outside 8-15 digits,
call IDs outside `/^[A-Za-z0-9._:@+-]{1,256}$/`, and SDP above 262,144 bytes.
Assert it returns a normalized copy rather than mutating its input.

- [ ] **Step 2: Write failing worker tests**

Load `service-worker.js` in `vm` with fake `chrome` and `fetch`. Test:

```js
test("opens one compact call window and focuses it on later clicks", async () => {
  const worker = createHarness();
  await worker.clickAction();
  await worker.clickAction();
  assert.equal(worker.chrome.windows.create.calls.length, 1);
  assert.equal(worker.chrome.windows.update.calls.length, 1);
  assert.deepEqual(worker.chrome.windows.create.calls[0], {
    url: "call-window.html", type: "popup", width: 380, height: 620, focused: true
  });
});

test("verifies and stores configuration but returns no api key", async () => {
  const worker = createHarness({
    fetchResponse: { ok: true, status: 200, body: { data: { Connected: true, LoggedIn: true, Name: "Suporte" } } }
  });
  const result = await worker.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  assert.equal(result.ok, true);
  assert.equal(JSON.stringify(result).includes("secret"), false);
  assert.equal(worker.savedConfig.apiKey, "secret");
  assert.equal(worker.fetch.calls[0].options.headers.apikey, "secret");
});
```

Also assert disconnected/logged-out status is rejected, 401 does not persist,
`CONFIG_GET` is sanitized, and `chrome.storage.local.setAccessLevel` receives
`TRUSTED_CONTEXTS` when available.

- [ ] **Step 3: Run RED**

Run:

```powershell
node --test extensions/wacalls-browser/tests/protocol.test.js extensions/wacalls-browser/tests/service-worker.test.js
```

Expected: FAIL because the protocol and worker factory are missing.

- [ ] **Step 4: Implement protocol validation**

Create a UMD-style `WaCallsProtocol` exposing `TYPES` and `validateMessage`.
Use an exact allowed-key set per message. `CONFIG_SAVE` accepts only `type`,
`apiUrl`, and non-empty `apiKey` up to 4096 characters. `CALL_WEBRTC` accepts
only `type`, valid `callId`, and `sdpOffer`. Return frozen normalized objects.

- [ ] **Step 5: Implement the testable worker factory**

Load shared modules with:

```js
importScripts("shared/core.js", "shared/protocol.js");
```

Expose `WaCallsWorker.createWorker({ chromeApi, fetchImpl })` for Node tests and
install it automatically in the real worker. Use storage keys
`wacallsConfiguration`, `wacallsClientId`, and `wacallsWindowId`.

`CONFIG_SAVE` must:

1. validate and normalize the exact origin;
2. confirm `chrome.permissions.contains({ origins: [pattern] })`;
3. fetch `GET /instance/status` with `Accept: application/json` and `apikey`;
4. require `connected && loggedIn`;
5. store URL, key, display name, both booleans, and ISO verification time;
6. return only `configured`, URL, name, booleans, and time.

Parse at most 1 MiB of response text. Map upstream errors through
`core.callErrorMessage`. Generate a client ID as `extension-${crypto.randomUUID()}`
once and persist it.

For the toolbar click, look up the stored window ID with `chrome.windows.get`.
Focus an existing window or create the exact 380x620 popup and store its ID.
Clear the stored ID from `chrome.windows.onRemoved`.

- [ ] **Step 6: Load shared scripts in the extension window**

Before `call-window.js`, load in order:

```html
<script src="shared/core.js"></script>
<script src="shared/protocol.js"></script>
<script src="call-controller.js"></script>
<script src="call-window.js"></script>
```

Create an empty `call-controller.js` temporarily so the manifest package remains
loadable. The controller is implemented in Task 5.

- [ ] **Step 7: Run GREEN and commit**

Run the two focused test files and then:

```powershell
node --test extensions/wacalls-browser/tests/*.test.js
```

Expected: all current tests pass.

```powershell
git add extensions/wacalls-browser
git commit -m "feat: secure standalone WACalls configuration"
```

### Task 4: Add the allowlisted Evolution call API boundary

**Files:**
- Modify: `extensions/wacalls-browser/service-worker.js`
- Modify: `extensions/wacalls-browser/tests/service-worker.test.js`

- [ ] **Step 1: Write failing call-operation tests**

Add worker tests that save a verified configuration, then assert:

```js
await worker.send({ type: "CALL_START", number: "5511999999999" });
await worker.send({ type: "CALL_WEBRTC", callId: "call-1", sdpOffer: "v=0\r\n" });
await worker.send({ type: "CALL_ACTIVE" });
await worker.send({ type: "CALL_END", callId: "call-1" });

assert.deepEqual(worker.fetch.calls.map(({ url, options }) => [url, options.method]), [
  ["https://api.example/call/start", "POST"],
  ["https://api.example/call/call-1/webrtc", "POST"],
  ["https://api.example/call/active", "GET"],
  ["https://api.example/call/call-1", "DELETE"]
]);
```

Verify every request contains the stored `apikey` and stable
`X-Call-Client-ID`, never a key supplied by the caller. Verify start returns a
sanitized `{ callId, direction, status }`, WebRTC returns only `sdpAnswer`,
active returns normalized call objects, and end returns one normalized call.
Verify unconfigured state, invalid IDs, unexpected response shapes, response
overflow, and arbitrary message fields fail closed.

- [ ] **Step 2: Run RED**

Run:

```powershell
node --test extensions/wacalls-browser/tests/service-worker.test.js
```

Expected: FAIL because call messages have no worker handlers.

- [ ] **Step 3: Implement one restricted request function**

Add `evolutionRequest(config, clientId, path, options)` that always derives the
base URL from verified storage and constructs headers internally:

```js
const headers = {
  Accept: "application/json",
  apikey: config.apiKey,
  "X-Call-Client-ID": clientId
};
if (options.body !== undefined) headers["Content-Type"] = "application/json";
```

Map operations with a closed switch. Encode validated call IDs with
`encodeURIComponent`. Use `{ keepalive: true }` for `CALL_END`. Do not accept a
generic path or request options from runtime messages.

- [ ] **Step 4: Sanitize each response shape**

Require non-empty `callId` after start, non-empty `sdpAnswer` no larger than
262,144 bytes after negotiation, an array from `body.calls || body.data || []`
for active state, and normalize end with `core.normalizeCall`. Return
`{ ok: false, error: <Portuguese message>, status }` from expected failures and
exclude raw bodies.

- [ ] **Step 5: Run GREEN and commit**

```powershell
node --test extensions/wacalls-browser/tests/service-worker.test.js
node --test extensions/wacalls-browser/tests/*.test.js
git add extensions/wacalls-browser/service-worker.js extensions/wacalls-browser/tests/service-worker.test.js
git commit -m "feat: allowlist standalone WACalls API operations"
```

Expected: all tests pass and no response contains the configured key.

### Task 5: Implement PCM worklets and the call controller

**Files:**
- Create: `extensions/wacalls-browser/audio-worklet.js`
- Modify: `extensions/wacalls-browser/call-controller.js`
- Create: `extensions/wacalls-browser/tests/audio-worklet.test.js`
- Create: `extensions/wacalls-browser/tests/call-controller.test.js`

- [ ] **Step 1: Write failing AudioWorklet tests**

Use `vm.runInNewContext` with fake `AudioWorkletProcessor`, `registerProcessor`,
and `sampleRate: 48000`. Assert registration of `wacalls-capture` and
`wacalls-playback`; capture produces 320 signed 16-bit samples per 20 ms frame;
capture stops emitting after `{ enabled: false }`; playback converts incoming
16 kHz PCM to nonzero 48 kHz output samples.

- [ ] **Step 2: Run AudioWorklet RED, implement, and run GREEN**

Run the test and verify missing-file failure. Then port the proven processors
from `manager/dist/assets/wacalls-audio-worklet.js` without importing Manager
code. Preserve linear resampling, clamping, a bounded two-second playback ring
buffer, and oldest-sample dropping on overflow.

Run:

```powershell
node --test extensions/wacalls-browser/tests/audio-worklet.test.js
```

Expected: all worklet tests pass.

- [ ] **Step 3: Write failing controller lifecycle tests**

Instantiate `WaCallsController.createController(deps)` with fake `sendMessage`,
`getUserMedia`, `AudioContext`, `AudioWorkletNode`, `RTCPeerConnection`, timers,
and `onState`. Assert:

- invalid phone rejects before `getUserMedia`;
- microphone resolves before `CALL_START` is sent;
- the peer creates `createDataChannel("pcm")` before its offer;
- local SDP is sent in `CALL_WEBRTC` and remote SDP is applied;
- captured frames are sent only while open, unmuted, and below 512 KiB buffered;
- received ArrayBuffers reach the playback worklet;
- polling `CALL_ACTIVE` updates ringing/connected/terminal state;
- mute toggles the audio track and posts `{ enabled: false/true }`;
- negotiation failure after start sends `CALL_END`;
- `end()` and `dispose()` clear timers, stop tracks, disconnect nodes, close
  AudioContext, data channel, and peer connection exactly once.

Use event-order assertions such as:

```js
assert.ok(events.indexOf("microphone") < events.indexOf("CALL_START"));
assert.deepEqual(messages[1], { type: "CALL_WEBRTC", callId: "call-1", sdpOffer: "v=0..." });
```

- [ ] **Step 4: Run controller RED**

```powershell
node --test extensions/wacalls-browser/tests/call-controller.test.js
```

Expected: FAIL because `createController` is missing.

- [ ] **Step 5: Implement the dependency-injected controller**

Expose `WaCallsController.createController(deps)` with:

```js
{
  start(rawNumber),
  toggleMute(),
  end(),
  dispose(),
  getState()
}
```

State contains `phase`, `number`, `callId`, `status`, `connectedAt`, `muted`,
`busy`, and `error`. Use phases `idle`, `preparing`, `active`, `ending`, and
`failed`. Load `audio-worklet.js` using `chrome.runtime.getURL`. Wait up to eight
seconds for ICE gathering and twelve seconds for the `pcm` channel. Poll active
calls every 1,500 ms and match the current `callId`. Start duration at the first
`connected` state. Cleanup must be idempotent and run from every terminal path.

- [ ] **Step 6: Run GREEN and commit**

```powershell
node --test extensions/wacalls-browser/tests/audio-worklet.test.js extensions/wacalls-browser/tests/call-controller.test.js
node --test extensions/wacalls-browser/tests/*.test.js
git add extensions/wacalls-browser
git commit -m "feat: add standalone WACalls browser audio"
```

Expected: all tests pass with no unhandled promise rejection.

### Task 6: Build the floating configuration, dialer, and active-call UI

**Files:**
- Modify: `extensions/wacalls-browser/call-window.html`
- Modify: `extensions/wacalls-browser/call-window.css`
- Modify: `extensions/wacalls-browser/call-window.js`
- Create: `extensions/wacalls-browser/tests/call-window.test.js`

- [ ] **Step 1: Write failing static UI and behavior tests**

Test the HTML for labeled `apiUrl`, `apiKey`, and `phone` inputs; buttons with
stable data attributes for save, settings, call, mute, and end; a `role="status"`
message region; and no inline script. Load `call-window.js` with a minimal fake
DOM and Chrome API to assert:

- unconfigured startup renders setup;
- configured startup renders only the dialer and does not restore the key;
- save normalizes the origin, requests exact-origin permission, and sends one
  `CONFIG_SAVE` message containing the submitted key;
- the key input is cleared immediately after the save attempt;
- permission denial does not send the key to the service worker;
- call, mute, and end buttons invoke the matching controller methods;
- active calls disable settings and phone editing;
- `pagehide` calls `controller.dispose({ endRemote: true })`.

- [ ] **Step 2: Run RED**

```powershell
node --test extensions/wacalls-browser/tests/call-window.test.js
```

Expected: FAIL because the final markup and bindings are missing.

- [ ] **Step 3: Implement accessible markup and compact styling**

Use one `<main>` with setup, dialer, and active-call sections. Keep labels
visible, set API key to `type="password"`, phone to `inputmode="tel"`, add
`aria-live="polite"` to status, and use real `<button>` elements. Style for a
380 px popup with responsive width, strong focus states, dark neutral colors,
green primary action, red hang-up action, and no external fonts or images.

- [ ] **Step 4: Implement UI bindings and rendering**

On `DOMContentLoaded`, request sanitized state with `CONFIG_GET`. During save:

```js
const origin = core.normalizeApiOrigin(apiUrlInput.value);
const granted = await chrome.permissions.request({
  origins: [core.permissionPattern(origin)]
});
if (!granted) throw new Error("Permissão para acessar a API não foi concedida.");
const apiKey = apiKeyInput.value;
apiKeyInput.value = "";
const result = await chrome.runtime.sendMessage({
  type: protocol.TYPES.CONFIG_SAVE,
  apiUrl: origin,
  apiKey
});
```

Create the controller only after configuration is verified. Render Portuguese
status labels for preparing, ringing, connected, ending, ended, and failed.
Update elapsed duration once per second from `connectedAt`; controller polling
remains independent. Never place the saved key back in the DOM.

- [ ] **Step 5: Run GREEN and commit**

```powershell
node --test extensions/wacalls-browser/tests/call-window.test.js
node --test extensions/wacalls-browser/tests/*.test.js
git add extensions/wacalls-browser
git commit -m "feat: add standalone WACalls floating dialer"
```

Expected: all UI and full extension tests pass.

### Task 7: Validate, package, document, and inspect the extension

**Files:**
- Create: `extensions/wacalls-browser/scripts/validate.mjs`
- Create: `extensions/wacalls-browser/scripts/package.ps1`
- Create: `extensions/wacalls-browser/tests/package.test.js`
- Create: `extensions/wacalls-browser/README.md`

- [ ] **Step 1: Write failing validation tests**

Create `tests/package.test.js` that runs `scripts/validate.mjs` and asserts exit
code zero for the real tree. Copy the extension to a temporary directory, add a
remote `<script src="https://example.com/x.js">`, and assert validation exits
nonzero. Also assert the manifest has no content script, Chatwoot reference,
required host permission, or remote URL.

- [ ] **Step 2: Run RED**

```powershell
node --test extensions/wacalls-browser/tests/package.test.js
```

Expected: FAIL because `scripts/validate.mjs` does not exist.

- [ ] **Step 3: Implement static validation**

The validator must parse the manifest, require MV3, verify every referenced
worker/HTML/script/style/worklet file, reject `http://` or `https://` script
sources, reject `eval`/`new Function`, reject `content_scripts`, reject
`host_permissions`, ensure only `storage` is required, and ensure runtime files
contain no case-insensitive `chatwoot` string. Print
`Extension validation passed.` only on success.

- [ ] **Step 4: Implement safe packaging**

`scripts/package.ps1` must resolve the extension root, `dist`, and artifacts
paths; verify both output paths remain children of the extension root before
removing them; run `npm test` and `npm run validate`; copy only manifest,
service worker, shared scripts, call window files, controller, and worklet into
`dist`; then create
`artifacts/evolution-go-wacalls-browser-0.1.0.zip` with `Compress-Archive`.

The ZIP root must contain `manifest.json`, not a wrapping directory.

- [ ] **Step 5: Document installation and operation**

README sections must cover Chrome `chrome://extensions`, Edge
`edge://extensions`, developer mode, Load unpacked from `dist`, initial URL/key
setup, exact-origin permission, phone with DDI/DDD, microphone permission,
mute/hang-up, HTTPS and loopback HTTP rules, `WEBRTC_PUBLIC_IP` and UDP firewall
requirements, local-storage credential limitation, upgrade, removal, and common
401/503/audio troubleshooting.

- [ ] **Step 6: Run complete verification**

```powershell
node --test extensions/wacalls-browser/tests/*.test.js
node extensions/wacalls-browser/scripts/validate.mjs
powershell -ExecutionPolicy Bypass -File extensions/wacalls-browser/scripts/package.ps1
```

Expected: all tests pass, validation prints `Extension validation passed.`,
`dist/manifest.json` exists, and the versioned ZIP exists.

- [ ] **Step 7: Inspect artifacts for secrets and forbidden coupling**

```powershell
rg -n -i "chatwoot|evogo\.melck|87\.99\.|apikey\s*[:=]\s*['\"][^'\"]+" extensions/wacalls-browser/dist
```

Expected: no matches. Then inspect `git status --short` and ensure only intended
extension files and the approved documentation changes are present.

- [ ] **Step 8: Commit the distributable extension**

```powershell
git add extensions/wacalls-browser docs/superpowers/specs/2026-06-28-standalone-wacalls-browser-extension-design.md docs/superpowers/plans/2026-06-28-standalone-wacalls-browser-extension.md
git commit -m "feat: package standalone WACalls browser extension"
```
