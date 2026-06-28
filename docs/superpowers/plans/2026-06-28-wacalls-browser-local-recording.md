# WACalls Browser Local Recording Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically record both sides of every standalone browser-extension call into an in-memory WebM/Opus file and offer an optional local download after termination.

**Architecture:** A new pure, dependency-injected recording module owns `MediaRecorder`, chunk accounting, format selection, and finalization. The existing call controller mixes microphone and remote playback into a `MediaStreamAudioDestinationNode`, coordinates recording with call cleanup, and exposes narrow recording metadata/Blob access to the window; the window performs only the explicit object-URL download.

**Tech Stack:** Manifest V3, vanilla JavaScript, Web Audio API, MediaRecorder, WebM/Opus, Node.js built-in test runner, PowerShell packaging.

---

## File Structure

- Create `extensions/wacalls-browser/recording.js`: isolated recorder state machine, MIME choice, chunks, size cap, Blob finalization, filename generation, and discard.
- Create `extensions/wacalls-browser/tests/recording.test.js`: fake-MediaRecorder unit tests.
- Modify `extensions/wacalls-browser/call-controller.js`: recording audio graph, automatic start, lifecycle finalization, state projection, and Blob access.
- Modify `extensions/wacalls-browser/tests/call-controller.test.js`: mixed-audio and lifecycle tests.
- Modify `extensions/wacalls-browser/call-window.html`: recording indicator, result card, download button, and consent notice.
- Modify `extensions/wacalls-browser/call-window.css`: recording/status/result styles.
- Modify `extensions/wacalls-browser/call-window.js`: recording render state and explicit Blob download.
- Modify `extensions/wacalls-browser/tests/call-window.test.js`: UI and download tests.
- Modify `extensions/wacalls-browser/manifest.json`: version `0.2.0` only; no new permission.
- Modify `extensions/wacalls-browser/scripts/validate.mjs`: require `recording.js` in runtime artifacts.
- Modify `extensions/wacalls-browser/scripts/package.ps1`: package `recording.js` and emit the new versioned ZIP.
- Modify `extensions/wacalls-browser/tests/manifest.test.js`: confirm no downloads permission and version.
- Modify `extensions/wacalls-browser/tests/package.test.js`: confirm recorder is included in the validated runtime tree.
- Modify `extensions/wacalls-browser/README.md`: automatic recording, privacy notice, optional download, format, discard, and limit.
- Regenerate `extensions/wacalls-browser/dist/` and `extensions/wacalls-browser/artifacts/evolution-go-wacalls-browser-0.2.0.zip`.

### Task 1: Build the isolated WebM recording state machine

**Files:**
- Create: `extensions/wacalls-browser/recording.js`
- Create: `extensions/wacalls-browser/tests/recording.test.js`

- [ ] **Step 1: Write fake-MediaRecorder test infrastructure**

Create `tests/recording.test.js` with a fake that records constructor options,
emits chunks, and emits a final `stop` event:

```js
"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const recordingModule = require("../recording.js");

class FakeMediaRecorder {
  static supported = new Set(["audio/webm;codecs=opus", "audio/webm"]);
  static isTypeSupported(value) { return this.supported.has(value); }
  constructor(stream, options) {
    this.stream = stream;
    this.options = options;
    this.state = "inactive";
    this.startTimeslice = 0;
    this.ondataavailable = null;
    this.onstop = null;
    this.onerror = null;
    FakeMediaRecorder.instances.push(this);
  }
  start(timeslice) { this.state = "recording"; this.startTimeslice = timeslice; }
  emit(data) { this.ondataavailable({ data }); }
  stop() {
    if (this.state === "inactive") return;
    this.state = "inactive";
    queueMicrotask(() => this.onstop());
  }
}
FakeMediaRecorder.instances = [];

function harness(overrides = {}) {
  FakeMediaRecorder.instances = [];
  FakeMediaRecorder.supported = new Set(overrides.supported || ["audio/webm;codecs=opus", "audio/webm"]);
  const states = [];
  const manager = recordingModule.createRecordingManager({
    MediaRecorder: FakeMediaRecorder,
    Blob,
    now: () => new Date("2026-06-28T12:34:56Z"),
    maxBytes: overrides.maxBytes || 250 * 1024 * 1024,
    onState(value) { states.push(value); },
  });
  return { manager, states };
}
```

- [ ] **Step 2: Write failing MIME, startup, and filename tests**

Add:

```js
test("prefers Opus WebM and starts in one-second chunks", () => {
  const { manager } = harness();
  manager.start({ id: "mixed-stream" }, "5511999999999");
  const recorder = FakeMediaRecorder.instances[0];
  assert.equal(recorder.options.mimeType, "audio/webm;codecs=opus");
  assert.equal(recorder.startTimeslice, 1000);
  assert.equal(manager.getState().status, "recording");
  assert.equal(manager.getState().filename, "evolution-call-5511999999999-20260628-123456.webm");
});

test("falls back to audio/webm", () => {
  const { manager } = harness({ supported: ["audio/webm"] });
  manager.start({}, "5511");
  assert.equal(FakeMediaRecorder.instances[0].options.mimeType, "audio/webm");
});
```

- [ ] **Step 3: Write failing finalization, size, error, and discard tests**

Cover exact behaviors:

```js
test("collects non-empty chunks and finalizes once", async () => {
  const { manager } = harness();
  manager.start({}, "5511");
  const recorder = FakeMediaRecorder.instances[0];
  recorder.emit(new Blob([]));
  recorder.emit(new Blob(["abc"], { type: "audio/webm;codecs=opus" }));
  const [first, second] = await Promise.all([manager.stop(), manager.stop()]);
  assert.equal(first.blob.size, 3);
  assert.equal(second.blob, first.blob);
  assert.equal(manager.getState().status, "ready");
  assert.equal(manager.getState().available, true);
});

test("stops at the configured memory limit", async () => {
  const { manager } = harness({ maxBytes: 3 });
  manager.start({}, "5511");
  FakeMediaRecorder.instances[0].emit(new Blob(["abcd"]));
  await new Promise(setImmediate);
  assert.equal(manager.getState().status, "ready");
  assert.equal(manager.getState().limitReached, true);
});

test("unsupported recording remains non-fatal", () => {
  const { manager } = harness({ supported: [] });
  assert.equal(manager.start({}, "5511"), false);
  assert.equal(manager.getState().status, "unavailable");
});

test("discard removes a completed Blob", async () => {
  const { manager } = harness();
  manager.start({}, "5511");
  FakeMediaRecorder.instances[0].emit(new Blob(["abc"]));
  await manager.stop();
  manager.discard();
  assert.equal(manager.getRecording(), null);
  assert.equal(manager.getState().status, "inactive");
});
```

Also assert an empty final Blob becomes `inactive`, constructor/start/runtime
errors become `failed`, and `getRecording()` returns `null` before `ready`.

- [ ] **Step 4: Run RED**

Run:

```powershell
node --test extensions/wacalls-browser/tests/recording.test.js
```

Expected: FAIL because `recording.js` does not exist.

- [ ] **Step 5: Implement `createRecordingManager`**

Use the same UMD/CommonJS pattern as `shared/core.js`. Export only:

```js
{ createRecordingManager }
```

The returned object is:

```js
{
  start(stream, normalizedPhone),
  stop(),
  discard(),
  getState(),
  getRecording()
}
```

Internal state uses `inactive`, `recording`, `finalizing`, `ready`,
`unavailable`, or `failed`; `bytes`, `available`, `filename`, `mimeType`,
`limitReached`, and `error` accompany it. Select MIME in the required order,
call `recorder.start(1000)`, ignore empty chunks, add chunk sizes without Blob
rebuilding, and stop once `bytes >= maxBytes`.

`stop()` must memoize one Promise before invoking `recorder.stop()`. The `stop`
handler creates `new Blob(chunks, { type: mimeType })`; a zero-byte result calls
`discard()`. Use digits-only phone and UTC components from `deps.now()` for the
filename. `discard()` clears recorder handlers, chunks, Blob, and metadata.

- [ ] **Step 6: Run GREEN and commit**

```powershell
node --test extensions/wacalls-browser/tests/recording.test.js
node --test extensions/wacalls-browser/tests/*.test.js
git add extensions/wacalls-browser/recording.js extensions/wacalls-browser/tests/recording.test.js
git commit -m "feat: add local WebM call recorder"
```

Expected: all recording and existing extension tests pass.

### Task 2: Mix both call directions and integrate recording lifecycle

**Files:**
- Modify: `extensions/wacalls-browser/call-window.html`
- Modify: `extensions/wacalls-browser/call-window.js`
- Modify: `extensions/wacalls-browser/call-controller.js`
- Modify: `extensions/wacalls-browser/tests/call-controller.test.js`

- [ ] **Step 1: Extend the controller harness before production changes**

In `tests/call-controller.test.js`, add a fake recording destination:

```js
const recordingDestination = new Node();
recordingDestination.stream = { id: "mixed-stream" };
context.createMediaStreamDestination = () => recordingDestination;
```

Make `Node.connect(target)` retain targets in `connections`. Inject a fake
recording manager through:

```js
recordingFactory(onState) {
  return {
    start(stream, phone) { recordingCalls.push(["start", stream, phone]); onState({ status: "recording", bytes: 0, available: false }); return true; },
    async stop() { recordingCalls.push(["stop"]); onState({ status: "ready", bytes: 3, available: true, filename: "call.webm" }); return { blob: new Blob(["abc"]), filename: "call.webm" }; },
    discard() { recordingCalls.push(["discard"]); onState({ status: "inactive", bytes: 0, available: false, filename: "" }); },
    getRecording() { return { blob: new Blob(["abc"]), filename: "call.webm" }; },
  };
}
```

- [ ] **Step 2: Write failing mixed graph and automatic-start tests**

Assert after `controller.start(...)`:

```js
assert.ok(source.connections.includes(recordingDestination));
assert.ok(playback.connections.includes(recordingDestination));
assert.deepEqual(recordingCalls[0], ["discard"]);
assert.deepEqual(recordingCalls[1], ["start", recordingDestination.stream, "5511999999999"]);
assert.ok(events.indexOf("channel:pcm") < events.indexOf("recording:start"));
assert.equal(controller.getState().recordingStatus, "recording");
```

- [ ] **Step 3: Write failing lifecycle-order tests**

For local `end()`, terminal polling, empty active list, negotiation failure after
the channel opens, and `dispose({ endRemote: true })`, record event order and
assert `recording:stop` occurs before track/context/peer cleanup. Assert:

```js
assert.deepEqual(controller.getRecording().filename, "call.webm");
```

Start a second call and assert `discard` precedes its microphone request. For a
recording manager returning `unavailable`, assert calling/mute/hang-up still
succeed and controller state exposes `recordingStatus: "unavailable"`.

- [ ] **Step 4: Run RED**

```powershell
node --test extensions/wacalls-browser/tests/call-controller.test.js
```

Expected: FAIL because no recording destination/factory/state exists.

- [ ] **Step 5: Load recording code before the controller**

In `call-window.html`, insert:

```html
<script src="recording.js"></script>
```

immediately before `call-controller.js`. In the browser controller factory in
`call-window.js`, inject:

```js
recordingFactory: function (onState) {
  return root.WaCallsRecording.createRecordingManager({
    MediaRecorder: root.MediaRecorder,
    Blob: root.Blob,
    now: function () { return new Date(); },
    maxBytes: 250 * 1024 * 1024,
    onState: onState,
  });
}
```

- [ ] **Step 6: Extend the audio graph**

In `setupAudio()`, create `recordingDestination`, connect both `source` and
`playback` to it in addition to their existing connections, and return it in the
audio object. Include it in disconnect cleanup.

- [ ] **Step 7: Integrate recording state and lifecycle**

Create one recording manager per controller. Add initial state:

```js
recordingStatus: "inactive",
recordingBytes: 0,
recordingAvailable: false,
recordingFilename: "",
recordingError: ""
```

Map recording callbacks into these fields and emit. At the beginning of
`start()`, call `recording.discard()`. After `waitForChannel(channel)`, call
`recording.start(audio.recordingDestination.stream, state.number)`.

Add `finalizeRecording()` that awaits `recording.stop()` without failing call
termination. Await it before every cleanup path where media may have opened.
Return `recording.getRecording()` from a new public `getRecording()` method.
On window disposal, finalize and then discard because no download UI remains.

- [ ] **Step 8: Run GREEN and commit**

```powershell
node --test extensions/wacalls-browser/tests/call-controller.test.js
node --test extensions/wacalls-browser/tests/*.test.js
git add extensions/wacalls-browser/call-controller.js extensions/wacalls-browser/call-window.html extensions/wacalls-browser/call-window.js extensions/wacalls-browser/tests/call-controller.test.js
git commit -m "feat: record both sides of browser calls"
```

Expected: lifecycle ordering tests and the full suite pass.

### Task 3: Add recording indicator and optional local download

**Files:**
- Modify: `extensions/wacalls-browser/call-window.html`
- Modify: `extensions/wacalls-browser/call-window.css`
- Modify: `extensions/wacalls-browser/call-window.js`
- Modify: `extensions/wacalls-browser/tests/call-window.test.js`

- [ ] **Step 1: Write failing accessible-markup tests**

Require:

```html
<div id="recordingIndicator" role="status">Gravando</div>
<section id="recordingPanel" hidden>
  <p>Gravação pronta</p>
  <span id="recordingSize"></span>
  <button type="button" data-action="download-recording">Baixar gravação</button>
</section>
```

and a visible notice containing `A gravação é automática` and responsibility
for consent. Assert the indicator has text, not color alone.

- [ ] **Step 2: Extend the window harness for downloads**

Add fake `document.createElement("a")`, `document.body.appendChild`, URL API,
and controller `getRecording()`. Track anchor clicks, names, created URLs, and
revocations:

```js
createObjectURL(blob) { urls.push(["create", blob]); return "blob:recording"; }
revokeObjectURL(value) { urls.push(["revoke", value]); }
```

- [ ] **Step 3: Write failing render and download tests**

Cover:

```js
harness.emitState({ phase: "active", status: "connected", recordingStatus: "recording", recordingBytes: 0, recordingAvailable: false });
assert.equal(harness.elements.recordingIndicator.hidden, false);

harness.emitState({ phase: "idle", status: "ended", recordingStatus: "ready", recordingBytes: 1536, recordingAvailable: true, recordingFilename: "call.webm" });
assert.equal(harness.elements.recordingPanel.hidden, false);
assert.equal(harness.elements.recordingSize.textContent, "1,5 KB");
await harness.elements["action:download-recording"].emit("click");
assert.equal(anchor.download, "call.webm");
assert.equal(anchor.href, "blob:recording");
assert.equal(anchor.clickCount, 1);
assert.deepEqual(urls.at(-1), ["revoke", "blob:recording"]);
```

Also assert `unavailable`/`failed` show a Portuguese recording warning without
hiding call controls, and starting another call hides the prior result card.

- [ ] **Step 4: Run RED**

```powershell
node --test extensions/wacalls-browser/tests/call-window.test.js
```

Expected: FAIL because recording elements and download behavior are absent.

- [ ] **Step 5: Implement UI structure and styles**

Place the recording indicator inside the active-call panel and the recording
result panel in the dialer. Style a red pulsing dot plus visible text, a compact
result card, and a secondary download button. Add the consent notice below the
dialer/configuration content. Keep all controls usable at 380 px.

- [ ] **Step 6: Implement rendering and download**

Render indicator only for `recordingStatus === "recording"` or `"finalizing"`.
Render the result panel only when `recordingAvailable === true`. Format bytes in
B, KB, or MB using one decimal and `pt-BR` decimal punctuation.

The click handler must:

```js
const result = controller.getRecording();
if (!result) throw new Error("A gravação não está disponível.");
const url = deps.urlApi.createObjectURL(result.blob);
const anchor = deps.document.createElement("a");
anchor.href = url;
anchor.download = result.filename;
deps.document.body.appendChild(anchor);
anchor.click();
anchor.remove();
deps.setTimeout(() => deps.urlApi.revokeObjectURL(url), 0);
```

Inject `urlApi: root.URL` into the browser app. Catch download failures and show
a Portuguese error without discarding the Blob.

- [ ] **Step 7: Run GREEN and commit**

```powershell
node --test extensions/wacalls-browser/tests/call-window.test.js
node --test extensions/wacalls-browser/tests/*.test.js
git add extensions/wacalls-browser/call-window.html extensions/wacalls-browser/call-window.css extensions/wacalls-browser/call-window.js extensions/wacalls-browser/tests/call-window.test.js
git commit -m "feat: offer local call recording downloads"
```

Expected: all window and full extension tests pass.

### Task 4: Version, document, package, and verify recording artifacts

**Files:**
- Modify: `extensions/wacalls-browser/manifest.json`
- Modify: `extensions/wacalls-browser/tests/manifest.test.js`
- Modify: `extensions/wacalls-browser/scripts/validate.mjs`
- Modify: `extensions/wacalls-browser/scripts/package.ps1`
- Modify: `extensions/wacalls-browser/tests/package.test.js`
- Modify: `extensions/wacalls-browser/README.md`
- Regenerate: `extensions/wacalls-browser/dist/`
- Replace: `extensions/wacalls-browser/artifacts/evolution-go-wacalls-browser-0.1.0.zip`
- Create: `extensions/wacalls-browser/artifacts/evolution-go-wacalls-browser-0.2.0.zip`

- [ ] **Step 1: Write failing manifest/package tests**

Assert:

```js
assert.equal(manifest.version, "0.2.0");
assert.deepEqual(manifest.permissions, ["storage"]);
assert.equal(manifest.permissions.includes("downloads"), false);
assert.match(fs.readFileSync(path.join(root, "call-window.html"), "utf8"), /recording\.js/);
```

In package validation tests, require `dist/recording.js` after packaging and
assert the validator fails if `recording.js` is removed from a copied runtime
tree.

- [ ] **Step 2: Run RED**

```powershell
node --test extensions/wacalls-browser/tests/manifest.test.js extensions/wacalls-browser/tests/package.test.js
```

Expected: FAIL on version and missing runtime recorder validation.

- [ ] **Step 3: Update manifest, validator, and packager**

Set manifest version to `0.2.0` without adding permissions. Add `recording.js` to
the validator runtime set and to `$RuntimeFiles` in `package.ps1`. Keep the ZIP
name derived from the manifest. The packager already safely recreates `dist/`
and `artifacts/`, so the old `0.1.0` ZIP is removed during regeneration.

- [ ] **Step 4: Update documentation**

README must state:

- every opened PCM call is recorded automatically;
- both sides are mixed locally;
- WebM/Opus format and MP4 exclusion;
- red visible indicator;
- optional explicit download after termination;
- memory-only retention and discard rules;
- 250 MiB limit;
- user responsibility for consent/privacy;
- recording failure does not block calling.

- [ ] **Step 5: Run the complete release verification**

```powershell
node --test extensions/wacalls-browser/tests/*.test.js
node extensions/wacalls-browser/scripts/validate.mjs
powershell -ExecutionPolicy Bypass -File extensions/wacalls-browser/scripts/package.ps1
```

Expected: all tests pass, validation prints `Extension validation passed.`,
`dist/recording.js` exists, and
`artifacts/evolution-go-wacalls-browser-0.2.0.zip` exists.

- [ ] **Step 6: Inspect runtime artifacts**

```powershell
Get-ChildItem -Recurse -File extensions/wacalls-browser/dist
Get-ChildItem -Recurse -File extensions/wacalls-browser/dist | Select-String -Pattern 'chatwoot|evogo\.melck|87\.99\.156\.233|429683C4C977415CAAFCCE10F7D57E11' -CaseSensitive:$false
git diff --check
git status --short --branch
```

Expected: runtime includes `recording.js`, secret/coupling scan has no matches,
diff check exits zero, and status contains only intended source, test, docs,
`dist`, and artifact changes.

- [ ] **Step 7: Manually verify current Chrome and Edge**

Load `extensions/wacalls-browser/dist`, place one real call, verify the red
indicator, both voices in the downloaded WebM, local mute silence with remote
audio retained, remote hang-up finalization, optional download, and discard on
the next call/window close. No automated test substitutes for this live media
check.

- [ ] **Step 8: Commit the release artifacts**

```powershell
git add extensions/wacalls-browser docs/superpowers/specs/2026-06-28-wacalls-browser-local-recording-design.md docs/superpowers/plans/2026-06-28-wacalls-browser-local-recording.md
git commit -m "feat: add automatic local call recording"
```

