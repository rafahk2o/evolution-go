"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const core = require("./wacalls-widget-core.js");

test("normalizes a WhatsApp destination", () => {
  assert.equal(core.normalizeNumber("+55 (11) 99999-9999"), "5511999999999");
  assert.equal(core.normalizeNumber("  "), "");
  assert.equal(core.normalizeNumber("abc"), "");
});

test("matches an instance name exactly instead of matching name prefixes", () => {
  assert.equal(core.matchesInstanceName("teste\nteste\nDesconectado\nStatus\nclose", "teste"), true);
  assert.equal(core.matchesInstanceName("teste2\nteste2\nDesconectado\nStatus\nclose", "teste"), false);
  assert.equal(core.matchesInstanceName("teste2\nteste2\nDesconectado\nStatus\nclose", "teste2"), true);
});

test("identifies every terminal call state", () => {
  assert.equal(core.isTerminalStatus("ended"), true);
  assert.equal(core.isTerminalStatus("rejected"), true);
  assert.equal(core.isTerminalStatus("failed"), true);
  assert.equal(core.isTerminalStatus("connected"), false);
});

test("maps HTTP call errors to actionable Portuguese messages", () => {
  assert.equal(core.callErrorMessage(409, {}), "Outro operador assumiu a chamada ou existe uma chamada em andamento.");
  assert.equal(core.callErrorMessage(503, { error: "custom" }), "custom");
  assert.equal(core.callErrorMessage(500, {}), "Falha inesperada ao controlar a chamada.");
});

test("normalizes API and SSE call payloads", () => {
  assert.deepEqual(
    core.normalizeCall({
      callId: "call-1",
      instanceId: "instance-1",
      direction: "incoming",
      status: "offered",
      peer: "5511999999999@s.whatsapp.net",
    }),
    {
      callId: "call-1",
      instanceId: "instance-1",
      clientId: "",
      direction: "incoming",
      status: "offered",
      peer: "5511999999999",
      timestamp: "",
    },
  );
});

test("parses SSE events split across arbitrary chunks", () => {
  const events = [];
  const parser = core.createSSEParser((event) => events.push(event));

  parser.push('event: call.incoming\r\ndata: {"callId":"abc"');
  parser.push(',"status":"offered"}\r\n\r\n');
  parser.push(': ping\n\n');

  assert.deepEqual(events, [
    {
      event: "call.incoming",
      data: { callId: "abc", status: "offered" },
    },
  ]);
});
