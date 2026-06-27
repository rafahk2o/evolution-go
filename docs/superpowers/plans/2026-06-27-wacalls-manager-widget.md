# WaCalls Manager Widget Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a complete browser call console to every connected instance card in the Evolution Go Manager.

**Architecture:** Keep the compiled React bundle untouched. Load a testable browser-neutral core, an injected Manager widget, and an AudioWorklet from `manager/dist/assets`; the widget consumes the existing authenticated call HTTP/SSE API and WebRTC PCM data channel.

**Tech Stack:** Vanilla JavaScript, WebRTC, Web Audio API/AudioWorklet, Fetch streaming SSE, Node.js built-in test runner.

---

### Task 1: Browser-neutral call helpers

**Files:**
- Create: `manager/dist/assets/wacalls-widget-core.js`
- Create: `manager/dist/assets/wacalls-widget-core.test.js`

- [ ] **Step 1: Write failing helper tests**

Cover number normalization, terminal-state detection, HTTP error labels, and SSE
events split across arbitrary chunks:

```js
const test = require("node:test");
const assert = require("node:assert/strict");
const core = require("./wacalls-widget-core.js");

test("normalizes a WhatsApp destination", () => {
  assert.equal(core.normalizeNumber("+55 (11) 99999-9999"), "5511999999999");
});

test("parses SSE across chunks", () => {
  const events = [];
  const parser = core.createSSEParser((event) => events.push(event));
  parser.push('event: call.incoming\ndata: {"callId":"abc"');
  parser.push('}\n\n');
  assert.deepEqual(events, [{ event: "call.incoming", data: { callId: "abc" } }]);
});
```

- [ ] **Step 2: Run tests and verify RED**

Run: `node --test manager/dist/assets/wacalls-widget-core.test.js`

Expected: FAIL because `wacalls-widget-core.js` does not exist.

- [ ] **Step 3: Implement the UMD helper module**

Export the exact API used by the widget:

```js
{
  normalizeNumber,
  isTerminalStatus,
  callErrorMessage,
  createSSEParser,
  normalizeCall
}
```

The module must attach to `window.WaCallsWidgetCore` in the browser and use
`module.exports` under Node.

- [ ] **Step 4: Run tests and verify GREEN**

Run: `node --test manager/dist/assets/wacalls-widget-core.test.js`

Expected: all helper tests PASS.

### Task 2: PCM AudioWorklet

**Files:**
- Create: `manager/dist/assets/wacalls-audio-worklet.js`
- Create: `manager/dist/assets/wacalls-audio-worklet.test.js`

- [ ] **Step 1: Write failing worklet contract test**

Load the script in a VM with a fake `AudioWorkletProcessor` and
`registerProcessor`, then assert registration of `wacalls-capture` and
`wacalls-playback`.

- [ ] **Step 2: Run test and verify RED**

Run: `node --test manager/dist/assets/wacalls-audio-worklet.test.js`

Expected: FAIL because the worklet does not exist.

- [ ] **Step 3: Implement capture and playback processors**

Capture must emit 20 ms `ArrayBuffer` frames containing signed 16-bit
little-endian mono PCM at 16 kHz. Playback must accept those buffers, resample to
the AudioContext rate, and play through a bounded ring buffer.

- [ ] **Step 4: Run test and verify GREEN**

Run: `node --test manager/dist/assets/wacalls-audio-worklet.test.js`

Expected: both processors register and PCM conversion tests PASS.

### Task 3: Manager card widget and complete call flow

**Files:**
- Create: `manager/dist/assets/wacalls-widget.js`
- Create: `manager/dist/assets/wacalls-widget.test.js`

- [ ] **Step 1: Write failing widget contract tests**

Verify the source exposes one card button marker, uses instance authentication,
sets `X-Call-Client-ID`, creates data channel `pcm`, consumes streaming SSE with
`fetch`, and calls all required endpoints:

```js
assert.match(source, /data-wacalls-button-for/);
assert.match(source, /X-Call-Client-ID/);
assert.match(source, /createDataChannel\("pcm"\)/);
assert.match(source, /\/call\/events/);
assert.match(source, /\/call\/start/);
assert.match(source, /\/webrtc/);
assert.match(source, /\/accept/);
assert.match(source, /\/reject/);
```

- [ ] **Step 2: Run test and verify RED**

Run: `node --test manager/dist/assets/wacalls-widget.test.js`

Expected: FAIL because `wacalls-widget.js` does not exist.

- [ ] **Step 3: Implement discovery, button, badge, and modal**

Reuse `evolution-auth`, load `/instance/all`, normalize cards, inject one phone
button per connected instance, monitor DOM mutations, and render a modal with
destination, state, duration, errors, mute, accept, reject, and end controls.

- [ ] **Step 4: Implement authenticated SSE and reconciliation**

Open one cancellable `fetch` stream per connected instance with `apikey` and
`X-Call-Client-ID`; parse it with the core helper, reconnect with bounded delay,
and call `/call/active` after connection/reconnection.

- [ ] **Step 5: Implement outgoing and incoming WebRTC flows**

For outgoing calls, get microphone permission, call `/call/start`, create the
`pcm` channel, negotiate `/call/:id/webrtc`, and follow SSE states. For incoming
calls, show the badge, negotiate/claim first, wait for the channel, then call
`/accept`; `/reject` must not open media.

- [ ] **Step 6: Implement terminal cleanup**

Stop tracks and audio nodes, close the AudioContext, data channel, peer
connection, timers, and modal state after `ended`, `rejected`, `failed`, local
end, instance disconnect, or confirmed modal close.

- [ ] **Step 7: Run widget tests and syntax checks**

Run:

```bash
node --test manager/dist/assets/wacalls-widget.test.js
node --check manager/dist/assets/wacalls-widget.js
```

Expected: PASS and no syntax errors.

### Task 4: Wire assets and verify delivery

**Files:**
- Modify: `manager/dist/index.html`
- Modify: `docs/wiki/guias-api/api-call.md`

- [ ] **Step 1: Write failing asset-wiring assertion**

Run a Node assertion that reads `manager/dist/index.html` and requires
`wacalls-widget-core.js` before `wacalls-widget.js`.

- [ ] **Step 2: Verify RED**

Expected: FAIL because neither script tag exists.

- [ ] **Step 3: Add deferred script tags**

Insert:

```html
<script defer src="/assets/wacalls-widget-core.js"></script>
<script defer src="/assets/wacalls-widget.js"></script>
```

The worklet is loaded dynamically by the widget and must not be a normal script
tag.

- [ ] **Step 4: Document Manager validation**

Add the icon/modal flow and HTTPS microphone requirement to the call API guide.

- [ ] **Step 5: Run complete verification**

Run:

```bash
node --test manager/dist/assets/wacalls-*.test.js
node --check manager/dist/assets/wacalls-widget-core.js
node --check manager/dist/assets/wacalls-audio-worklet.js
node --check manager/dist/assets/wacalls-widget.js
git diff --check
```

Expected: all tests pass, all scripts parse, and the diff has no whitespace
errors.
