"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const core = require("../shared/core.js");

test("accepts HTTPS origins and loopback HTTP only", () => {
  assert.equal(core.normalizeApiOrigin("https://api.example.com/"), "https://api.example.com");
  assert.equal(core.normalizeApiOrigin("http://localhost:8080"), "http://localhost:8080");
  assert.equal(core.normalizeApiOrigin("http://127.0.0.1:8080/"), "http://127.0.0.1:8080");
  assert.throws(() => core.normalizeApiOrigin("http://api.example.com"), /HTTPS/);
  assert.throws(() => core.normalizeApiOrigin("https://user:pass@api.example.com"), /credenciais/);
  assert.throws(() => core.normalizeApiOrigin("https://api.example.com/v1"), /origem/);
  assert.throws(() => core.normalizeApiOrigin("https://api.example.com?x=1"), /origem/);
});

test("creates an exact host permission pattern", () => {
  assert.equal(core.permissionPattern("https://api.example.com:8443"), "https://api.example.com:8443/*");
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
    instanceName: "Suporte",
  });
  assert.deepEqual(core.normalizeInstanceStatus({ data: { connected: false, loggedIn: false, name: "" } }), {
    connected: false,
    loggedIn: false,
    instanceName: "",
  });
});

test("normalizes calls and terminal states", () => {
  assert.deepEqual(core.normalizeCall({ callId: "c1", status: "connected", peer: "5511@s.whatsapp.net" }), {
    callId: "c1",
    direction: "",
    status: "connected",
    peer: "5511",
  });
  assert.equal(core.isTerminalStatus("ended"), true);
  assert.equal(core.isTerminalStatus("rejected"), true);
  assert.equal(core.isTerminalStatus("failed"), true);
  assert.equal(core.isTerminalStatus("connected"), false);
});

test("maps errors to concise Portuguese messages", () => {
  assert.equal(core.callErrorMessage(401, {}), "API key inválida ou expirada.");
  assert.equal(core.callErrorMessage(409, {}), "Já existe uma chamada em andamento para este navegador.");
  assert.equal(core.callErrorMessage(422, {}), "Número ou negociação WebRTC inválidos.");
  assert.equal(core.callErrorMessage(503, {}), "A instância está desconectada ou a mídia está indisponível.");
  assert.equal(core.callErrorMessage(504, {}), "A negociação WebRTC excedeu o tempo limite.");
  assert.equal(core.callErrorMessage(0, {}, new TypeError("Failed to fetch")), "Não foi possível acessar a Evolution GO.");
  assert.equal(core.callErrorMessage(500, { error: "erro específico" }), "erro específico");
  assert.equal(core.callErrorMessage(500, { error: "x".repeat(241) }), "Falha inesperada ao controlar a chamada.");
});
