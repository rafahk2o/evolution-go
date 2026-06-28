"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

function source() {
  return fs.readFileSync(path.join(__dirname, "wacalls-widget.js"), "utf8");
}

test("injects one identifiable call button per instance card", () => {
  const script = source();
  assert.match(script, /data-wacalls-button-for/);
  assert.match(script, /classList\.add\("wc-action-bar"\)/);
  assert.match(script, /MutationObserver/);
  assert.match(script, /Chamada recebida/);
});

test("call and Chatwoot injectors select exact instance cards", () => {
  const calls = source();
  const chatwoot = fs.readFileSync(path.join(__dirname, "chatwoot-connector.js"), "utf8");

  assert.match(calls, /core\.matchesInstanceName\(text, instance\.name\)/);
  assert.match(chatwoot, /cardCore\.matchesInstanceName\(text, instance\.name\)/);
  assert.doesNotMatch(calls, /text\.indexOf\(instance\.name\)/);
  assert.doesNotMatch(chatwoot, /text\.indexOf\(instance\.name\)/);
});

test("instance action bars reserve enough width for the disconnect label", () => {
  const calls = source();
  const chatwoot = fs.readFileSync(path.join(__dirname, "chatwoot-connector.js"), "utf8");

  [calls, chatwoot].forEach((script) => {
    assert.match(script, /flex:1 1 116px!important/);
    assert.match(script, /min-width:116px!important/);
    assert.match(script, /white-space:nowrap!important/);
    assert.match(script, /text-overflow:ellipsis!important/);
    assert.match(script, /flex:0 0 38px!important/);
  });
});

test("authenticates call ownership and streams SSE through fetch", () => {
  const script = source();
  assert.match(script, /X-Call-Client-ID/);
  assert.match(script, /evolution-wacalls-client-id/);
  assert.match(script, /\/call\/events/);
  assert.match(script, /response\.body\.getReader\(\)/);
  assert.doesNotMatch(script, /new EventSource/);
});

test("implements every call control endpoint", () => {
  const script = source();
  assert.match(script, /\/call\/start/);
  assert.match(script, /\/call\/active/);
  assert.match(script, /\/webrtc/);
  assert.match(script, /\/accept/);
  assert.match(script, /\/reject/);
  assert.match(script, /method:\s*"DELETE"/);
});

test("negotiates the PCM channel and browser audio", () => {
  const script = source();
  assert.match(script, /createDataChannel\("pcm"\)/);
  assert.match(script, /wacalls-audio-worklet\.js/);
  assert.match(script, /getUserMedia\(\{\s*audio:\s*true/);
  assert.match(script, /new AudioWorkletNode/);
});

test("obtains microphone access before starting an outgoing WhatsApp call", () => {
  const script = source();
  const start = script.slice(script.indexOf("async function startCall"), script.indexOf("async function acceptCall"));
  assert.ok(start.indexOf("setupAudio()") >= 0, "startCall must request browser audio");
  assert.ok(start.indexOf("setupAudio()") < start.indexOf('"/call/start"'), "microphone must be ready before /call/start");
});

test("loads the core before the widget without loading the worklet as a page script", () => {
  const html = fs.readFileSync(path.join(__dirname, "..", "index.html"), "utf8");
  const coreIndex = html.indexOf('/assets/wacalls-widget-core.js');
  const widgetIndex = html.indexOf('/assets/wacalls-widget.js');
  assert.ok(coreIndex >= 0, "index.html must load the widget core");
  assert.ok(widgetIndex > coreIndex, "index.html must load the widget after its core");
  assert.equal(html.includes('/assets/wacalls-audio-worklet.js'), false);
});
