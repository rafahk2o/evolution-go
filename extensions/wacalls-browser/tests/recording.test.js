"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const recordingModule = require("../recording.js");

class FakeMediaRecorder {
  static supported = new Set();
  static instances = [];
  static isTypeSupported(value) { return this.supported.has(value); }
  constructor(stream, options) {
    this.stream = stream;
    this.options = options;
    this.state = "inactive";
    FakeMediaRecorder.instances.push(this);
  }
  start(timeslice) { this.state = "recording"; this.startTimeslice = timeslice; }
  emit(data) { this.ondataavailable({ data }); }
  stop() { if (this.state === "inactive") return; this.state = "inactive"; queueMicrotask(() => this.onstop()); }
  runtimeError() { this.state = "inactive"; this.onerror({ error: new Error("runtime boom") }); queueMicrotask(() => this.onstop()); }
}

function harness(options = {}) {
  FakeMediaRecorder.instances = [];
  FakeMediaRecorder.supported = new Set(options.supported === undefined ? ["audio/webm;codecs=opus", "audio/webm"] : options.supported);
  const states = [];
  const manager = recordingModule.createRecordingManager({
    MediaRecorder: options.MediaRecorder === undefined ? FakeMediaRecorder : options.MediaRecorder,
    Blob,
    now: () => new Date("2026-06-28T12:34:56Z"),
    maxBytes: options.maxBytes || 250 * 1024 * 1024,
    onState(value) { states.push(value); },
  });
  return { manager, states };
}

test("prefers Opus WebM and starts in one-second chunks", () => {
  const { manager } = harness();
  assert.equal(manager.start({ id: "mixed" }, "5511999999999"), true);
  const recorder = FakeMediaRecorder.instances[0];
  assert.equal(recorder.options.mimeType, "audio/webm;codecs=opus");
  assert.equal(recorder.startTimeslice, 1000);
  assert.equal(manager.getState().filename, "evolution-call-5511999999999-20260628-123456.webm");
});

test("falls back to plain WebM and reports unsupported browsers", () => {
  const fallback = harness({ supported: ["audio/webm"] });
  fallback.manager.start({}, "5511");
  assert.equal(FakeMediaRecorder.instances[0].options.mimeType, "audio/webm");
  const unsupported = harness({ supported: [] });
  assert.equal(unsupported.manager.start({}, "5511"), false);
  assert.equal(unsupported.manager.getState().status, "unavailable");
});

test("collects non-empty chunks and finalizes idempotently", async () => {
  const { manager } = harness();
  manager.start({}, "5511");
  const recorder = FakeMediaRecorder.instances[0];
  recorder.emit(new Blob([]));
  recorder.emit(new Blob(["abc"]));
  const [first, second] = await Promise.all([manager.stop(), manager.stop()]);
  assert.equal(first.blob.size, 3);
  assert.equal(second.blob, first.blob);
  assert.equal(manager.getState().status, "ready");
});

test("stops at the memory limit and marks the result", async () => {
  const { manager } = harness({ maxBytes: 3 });
  manager.start({}, "5511");
  FakeMediaRecorder.instances[0].emit(new Blob(["abcd"]));
  await new Promise(setImmediate);
  assert.equal(manager.getState().status, "ready");
  assert.equal(manager.getState().limitReached, true);
});

test("discards empty and explicitly discarded recordings", async () => {
  const empty = harness();
  empty.manager.start({}, "5511");
  assert.equal(await empty.manager.stop(), null);
  assert.equal(empty.manager.getState().status, "inactive");

  const complete = harness();
  complete.manager.start({}, "5511");
  FakeMediaRecorder.instances[0].emit(new Blob(["abc"]));
  await complete.manager.stop();
  complete.manager.discard();
  assert.equal(complete.manager.getRecording(), null);
});

test("construction and runtime failures remain isolated", async () => {
  class BrokenRecorder extends FakeMediaRecorder { constructor() { super(); throw new Error("boom"); } }
  BrokenRecorder.isTypeSupported = () => true;
  const broken = harness({ MediaRecorder: BrokenRecorder });
  assert.equal(broken.manager.start({}, "5511"), false);
  assert.equal(broken.manager.getState().status, "failed");

  const runtime = harness();
  runtime.manager.start({}, "5511");
  FakeMediaRecorder.instances[0].runtimeError();
  await new Promise(setImmediate);
  assert.equal(runtime.manager.getState().status, "failed");
  assert.match(runtime.manager.getState().error, /runtime boom/);
});
