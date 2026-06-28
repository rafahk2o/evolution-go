"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const protocol = require("../shared/protocol.js");

test("accepts only known message types and exact fields", () => {
  assert.deepEqual(protocol.validateMessage({ type: "CONFIG_GET" }), { type: "CONFIG_GET" });
  assert.throws(() => protocol.validateMessage({ type: "UNKNOWN" }), /não suportada/);
  assert.throws(() => protocol.validateMessage({ type: "CALL_ACTIVE", url: "https://evil.example" }), /Campo não permitido/);
  assert.throws(() => protocol.validateMessage({ type: "CALL_START", number: "5511999999999", headers: {} }), /Campo não permitido/);
});

test("allows an API key only in configuration save", () => {
  assert.deepEqual(protocol.validateMessage({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" }), {
    type: "CONFIG_SAVE",
    apiUrl: "https://api.example",
    apiKey: "secret",
  });
  assert.throws(() => protocol.validateMessage({ type: "CALL_ACTIVE", apiKey: "secret" }), /Campo não permitido/);
  assert.throws(() => protocol.validateMessage({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "" }), /API key/);
});

test("validates call operation payload limits", () => {
  assert.deepEqual(protocol.validateMessage({ type: "CALL_START", number: "+55 (11) 99999-9999" }), {
    type: "CALL_START",
    number: "5511999999999",
  });
  assert.throws(() => protocol.validateMessage({ type: "CALL_START", number: "123" }), /DDI e DDD/);
  assert.throws(() => protocol.validateMessage({ type: "CALL_END", callId: "bad/id" }), /call ID/);
  assert.throws(() => protocol.validateMessage({ type: "CALL_WEBRTC", callId: "call-1", sdpOffer: "x".repeat(262145) }), /SDP/);
});

test("returns frozen normalized messages", () => {
  const input = { type: "CALL_END", callId: " call-1 " };
  const result = protocol.validateMessage(input);
  assert.deepEqual(result, { type: "CALL_END", callId: "call-1" });
  assert.equal(Object.isFrozen(result), true);
  assert.equal(input.callId, " call-1 ");
});
