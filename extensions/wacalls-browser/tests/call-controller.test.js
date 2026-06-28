"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const controllerModule = require("../call-controller.js");

function createHarness(overrides) {
  overrides = overrides || {};
  const events = [];
  const messages = [];
  const intervals = [];
  const track = { enabled: true, stopped: 0, stop() { this.stopped += 1; } };
  const stream = { getTracks: () => [track], getAudioTracks: () => [track] };
  class Node {
    constructor() { this.port = { messages: [], onmessage: null, postMessage: (value) => this.port.messages.push(value) }; this.disconnected = 0; }
    connect() {}
    disconnect() { this.disconnected += 1; }
  }
  const source = new Node();
  const capture = new Node();
  const playback = new Node();
  const silent = new Node();
  silent.gain = { value: 1 };
  const context = {
    destination: {},
    audioWorklet: { async addModule(url) { events.push("worklet:" + url); } },
    async resume() {},
    createMediaStreamSource() { return source; },
    createGain() { return silent; },
    closeCount: 0,
    async close() { this.closeCount += 1; },
  };
  class AudioContext { constructor() { return context; } }
  class AudioWorkletNode { constructor(_context, name) { return name === "wacalls-capture" ? capture : playback; } }
  const channel = {
    readyState: "open",
    binaryType: "",
    bufferedAmount: 0,
    sent: [],
    closeCount: 0,
    send(value) { this.sent.push(value); },
    close() { this.closeCount += 1; },
    addEventListener() {},
    onmessage: null,
  };
  const peer = {
    iceGatheringState: "complete",
    connectionState: "connected",
    localDescription: null,
    remoteDescription: null,
    closeCount: 0,
    createDataChannel(name) { events.push("channel:" + name); return channel; },
    async createOffer() { events.push("offer"); return { type: "offer", sdp: "v=0..." }; },
    async setLocalDescription(value) { this.localDescription = value; },
    async setRemoteDescription(value) { this.remoteDescription = value; },
    addEventListener() {},
    removeEventListener() {},
    close() { this.closeCount += 1; },
  };
  const responses = overrides.responses || [
    { ok: true, callId: "call-1", direction: "outgoing", status: "starting" },
    { ok: true, sdpAnswer: "v=0 answer" },
  ];
  const deps = {
    async sendMessage(message) { messages.push(message); events.push(message.type); return responses.shift(); },
    mediaDevices: { async getUserMedia() { events.push("microphone"); if (overrides.micError) throw overrides.micError; return stream; } },
    AudioContext,
    AudioWorkletNode,
    RTCPeerConnection: class { constructor() { return peer; } },
    runtimeGetURL: (value) => "extension://" + value,
    setTimeout: (fn) => { fn(); return 1; },
    clearTimeout() {},
    setInterval(fn) { intervals.push(fn); return intervals.length; },
    clearInterval(id) { intervals[id - 1] = null; },
    now: () => 1000,
    onState(value) { events.push("state:" + value.phase + ":" + value.status); },
  };
  return {
    controller: controllerModule.createController(deps),
    events, messages, intervals, track, stream, context, capture, playback, channel, peer,
  };
}

test("rejects invalid phone before requesting the microphone", async () => {
  const harness = createHarness();
  await assert.rejects(() => harness.controller.start("123"), /DDI e DDD/);
  assert.equal(harness.events.includes("microphone"), false);
});

test("starts media before the call and negotiates the pcm channel", async () => {
  const harness = createHarness();
  await harness.controller.start("+55 (11) 99999-9999");
  assert.ok(harness.events.indexOf("microphone") < harness.events.indexOf("CALL_START"));
  assert.ok(harness.events.indexOf("channel:pcm") < harness.events.indexOf("offer"));
  assert.deepEqual(harness.messages[1], { type: "CALL_WEBRTC", callId: "call-1", sdpOffer: "v=0..." });
  assert.deepEqual(harness.peer.remoteDescription, { type: "answer", sdp: "v=0 answer" });

  const frame = new ArrayBuffer(640);
  harness.capture.port.onmessage({ data: frame });
  assert.equal(harness.channel.sent.length, 1);
  const incoming = new ArrayBuffer(640);
  harness.channel.onmessage({ data: incoming });
  assert.equal(harness.playback.port.messages[0], incoming);
});

test("mute controls both the track and capture worklet", async () => {
  const harness = createHarness();
  await harness.controller.start("5511999999999");
  assert.equal(harness.controller.toggleMute(), true);
  assert.equal(harness.track.enabled, false);
  assert.deepEqual(harness.capture.port.messages.at(-1), { enabled: false });
  assert.equal(harness.controller.toggleMute(), false);
  assert.equal(harness.track.enabled, true);
});

test("polling updates connected state and terminal cleanup", async () => {
  const harness = createHarness({ responses: [
    { ok: true, callId: "call-1", direction: "outgoing", status: "starting" },
    { ok: true, sdpAnswer: "v=0 answer" },
    { ok: true, calls: [{ callId: "call-1", status: "connected", peer: "5511" }] },
    { ok: true, calls: [{ callId: "call-1", status: "ended", peer: "5511" }] },
  ] });
  await harness.controller.start("5511999999999");
  await harness.intervals[0]();
  assert.equal(harness.controller.getState().status, "connected");
  assert.equal(harness.controller.getState().connectedAt, 1000);
  await harness.intervals[0]();
  assert.equal(harness.track.stopped, 1);
  assert.equal(harness.peer.closeCount, 1);
  assert.equal(harness.channel.closeCount, 1);
});

test("an empty active list after dialing is treated as remote termination", async () => {
  const harness = createHarness({ responses: [
    { ok: true, callId: "call-1", direction: "outgoing", status: "starting" },
    { ok: true, sdpAnswer: "v=0 answer" },
    { ok: true, calls: [] },
  ] });
  await harness.controller.start("5511999999999");
  await harness.intervals[0]();
  assert.equal(harness.controller.getState().status, "ended");
  assert.equal(harness.track.stopped, 1);
  assert.equal(harness.peer.closeCount, 1);
});

test("negotiation failure ends the remote call and cleanup is idempotent", async () => {
  const harness = createHarness({ responses: [
    { ok: true, callId: "call-1", direction: "outgoing", status: "starting" },
    { ok: false, error: "Falha WebRTC" },
    { ok: true, call: { callId: "call-1", status: "ended" } },
  ] });
  await assert.rejects(() => harness.controller.start("5511999999999"), /Falha WebRTC/);
  assert.equal(harness.messages.at(-1).type, "CALL_END");
  await harness.controller.dispose({ endRemote: true });
  assert.equal(harness.track.stopped, 1);
  assert.equal(harness.context.closeCount, 1);
});
