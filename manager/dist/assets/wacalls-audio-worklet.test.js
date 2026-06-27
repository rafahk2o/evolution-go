"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

function loadWorklet() {
  const registered = {};
  class FakePort {
    constructor() {
      this.messages = [];
      this.onmessage = null;
    }

    postMessage(message) {
      this.messages.push(message);
    }
  }

  class FakeAudioWorkletProcessor {
    constructor() {
      this.port = new FakePort();
    }
  }

  const source = fs.readFileSync(path.join(__dirname, "wacalls-audio-worklet.js"), "utf8");
  vm.runInNewContext(source, {
    AudioWorkletProcessor: FakeAudioWorkletProcessor,
    registerProcessor(name, processor) {
      registered[name] = processor;
    },
    sampleRate: 48000,
    Float32Array,
    Int16Array,
    ArrayBuffer,
    Math,
  });
  return registered;
}

test("registers capture and playback processors", () => {
  const registered = loadWorklet();
  assert.equal(typeof registered["wacalls-capture"], "function");
  assert.equal(typeof registered["wacalls-playback"], "function");
});

test("capture emits 20 ms signed 16-bit PCM frames at 16 kHz", () => {
  const Capture = loadWorklet()["wacalls-capture"];
  const capture = new Capture();
  const samples = new Float32Array(128).fill(0.5);

  for (let index = 0; index < 8; index += 1) {
    capture.process([[samples]], [], {});
  }

  assert.ok(capture.port.messages.length >= 1);
  const frame = capture.port.messages[0];
  assert.ok(frame instanceof ArrayBuffer);
  assert.equal(frame.byteLength, 320 * 2);
  assert.ok(new Int16Array(frame)[0] > 16000);
});

test("playback converts incoming 16 kHz PCM into output samples", () => {
  const Playback = loadWorklet()["wacalls-playback"];
  const playback = new Playback();
  const pcm = new Int16Array(320).fill(16384);
  playback.port.onmessage({ data: pcm.buffer });

  const output = new Float32Array(128);
  playback.process([], [[output]], {});

  assert.ok(output.some((sample) => sample > 0.45));
});

