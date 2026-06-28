"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const workerModule = require("../service-worker.js");

function event() {
  const listeners = [];
  return {
    addListener(listener) { listeners.push(listener); },
    async emit() { return Promise.all(listeners.map((listener) => listener(...arguments))); },
  };
}

function createHarness(options) {
  options = options || {};
  const store = {};
  const calls = { create: [], update: [], fetch: [], access: [] };
  const actionClicked = event();
  const windowRemoved = event();
  const runtimeMessage = event();
  let currentWindow = null;
  const chromeApi = {
    action: { onClicked: actionClicked },
    runtime: { onMessage: runtimeMessage, lastError: null },
    windows: {
      onRemoved: windowRemoved,
      async create(value) { calls.create.push(value); currentWindow = { id: 17 }; return currentWindow; },
      async get(id) { if (!currentWindow || currentWindow.id !== id) throw new Error("missing"); return currentWindow; },
      async update(id, value) { calls.update.push({ id, value }); return { id, ...value }; },
    },
    permissions: {
      async contains() { return options.permission !== false; },
    },
    storage: {
      local: {
        async get(key) { return { [key]: store[key] }; },
        async set(values) { Object.assign(store, values); },
        async remove(key) { delete store[key]; },
        async setAccessLevel(value) { calls.access.push(value); },
      },
    },
  };
  const defaultResponse = options.fetchResponse || {
    ok: true,
    status: 200,
    body: { data: { Connected: true, LoggedIn: true, InstanceName: "Suporte", Name: "Teste" } },
  };
  const responses = options.fetchResponses || null;
  async function fetchImpl(url, init) {
    const response = responses ? responses[calls.fetch.length] : defaultResponse;
    calls.fetch.push({ url, options: init });
    return {
      ok: response.ok,
      status: response.status,
      async text() { return JSON.stringify(response.body); },
    };
  }
  const worker = workerModule.createWorker({
    chromeApi,
    fetchImpl,
    cryptoApi: { randomUUID: () => "client-uuid" },
    now: () => new Date("2026-06-28T12:00:00Z"),
  });
  worker.install();
  return {
    chromeApi,
    calls,
    store,
    worker,
    clickAction: () => actionClicked.emit(),
    send: (message) => worker.handleMessage(message),
  };
}

test("opens one compact call window and focuses it on later clicks", async () => {
  const harness = createHarness();
  await harness.clickAction();
  await harness.clickAction();
  assert.equal(harness.calls.create.length, 1);
  assert.equal(harness.calls.update.length, 1);
  assert.deepEqual(harness.calls.create[0], {
    url: "call-window.html",
    type: "popup",
    width: 380,
    height: 620,
    focused: true,
  });
});

test("verifies and stores configuration but returns no API key", async () => {
  const harness = createHarness();
  const result = await harness.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  assert.equal(result.ok, true);
  assert.equal(JSON.stringify(result).includes("secret"), false);
  assert.equal(harness.store.wacallsConfiguration.apiKey, "secret");
  assert.equal(harness.calls.fetch[0].url, "https://api.example/instance/status");
  assert.equal(harness.calls.fetch[0].options.headers.apikey, "secret");
  assert.deepEqual(harness.calls.access[0], { accessLevel: "TRUSTED_CONTEXTS" });
});

test("returns only sanitized configuration state", async () => {
  const harness = createHarness();
  await harness.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  const result = await harness.send({ type: "CONFIG_GET" });
  assert.deepEqual(result, {
    ok: true,
    configured: true,
    apiUrl: "https://api.example",
    instanceName: "Suporte",
    connected: true,
    loggedIn: true,
    lastVerifiedAt: "2026-06-28T12:00:00.000Z",
  });
});

test("does not persist disconnected or unauthorized configuration", async () => {
  const disconnected = createHarness({
    fetchResponse: { ok: true, status: 200, body: { data: { Connected: false, LoggedIn: false, Name: "" } } },
  });
  const disconnectedResult = await disconnected.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  assert.equal(disconnectedResult.ok, false);
  assert.equal(disconnected.store.wacallsConfiguration, undefined);

  const unauthorized = createHarness({ fetchResponse: { ok: false, status: 401, body: {} } });
  const unauthorizedResult = await unauthorized.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  assert.equal(unauthorizedResult.ok, false);
  assert.equal(unauthorized.store.wacallsConfiguration, undefined);
});

test("requires the exact origin permission before sending credentials", async () => {
  const harness = createHarness({ permission: false });
  const result = await harness.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  assert.equal(result.ok, false);
  assert.equal(harness.calls.fetch.length, 0);
});

test("maps every call operation to an allowlisted Evolution route", async () => {
  const harness = createHarness({ fetchResponses: [
    { ok: true, status: 200, body: { data: { Connected: true, LoggedIn: true, Name: "Suporte" } } },
    { ok: true, status: 201, body: { callId: "call-1", direction: "outgoing", status: "starting", ignored: "x" } },
    { ok: true, status: 200, body: { sdpAnswer: "v=0\r\nanswer", ignored: "x" } },
    { ok: true, status: 200, body: { calls: [{ callId: "call-1", status: "connected", peer: "5511@s.whatsapp.net" }] } },
    { ok: true, status: 200, body: { callId: "call-1", status: "ended", peer: "5511@s.whatsapp.net" } },
  ] });
  await harness.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  const started = await harness.send({ type: "CALL_START", number: "5511999999999" });
  const negotiated = await harness.send({ type: "CALL_WEBRTC", callId: "call-1", sdpOffer: "v=0\r\n" });
  const active = await harness.send({ type: "CALL_ACTIVE" });
  const ended = await harness.send({ type: "CALL_END", callId: "call-1" });

  assert.deepEqual(harness.calls.fetch.slice(1).map(({ url, options }) => [url, options.method]), [
    ["https://api.example/call/start", "POST"],
    ["https://api.example/call/call-1/webrtc", "POST"],
    ["https://api.example/call/active", "GET"],
    ["https://api.example/call/call-1", "DELETE"],
  ]);
  harness.calls.fetch.slice(1).forEach(({ options }) => {
    assert.equal(options.headers.apikey, "secret");
    assert.equal(options.headers["X-Call-Client-ID"], "extension-client-uuid");
  });
  assert.deepEqual(started, { ok: true, callId: "call-1", direction: "outgoing", status: "starting" });
  assert.deepEqual(negotiated, { ok: true, sdpAnswer: "v=0\r\nanswer" });
  assert.deepEqual(active, { ok: true, calls: [{ callId: "call-1", direction: "", status: "connected", peer: "5511" }] });
  assert.deepEqual(ended, { ok: true, call: { callId: "call-1", direction: "", status: "ended", peer: "5511" } });
  assert.equal(harness.calls.fetch[4].options.keepalive, true);
});

test("fails closed when calls are unconfigured or responses are malformed", async () => {
  const unconfigured = createHarness();
  assert.deepEqual(await unconfigured.send({ type: "CALL_ACTIVE" }), {
    ok: false,
    error: "Configure e teste a Evolution GO antes de ligar.",
  });

  const malformed = createHarness({ fetchResponses: [
    { ok: true, status: 200, body: { data: { Connected: true, LoggedIn: true, Name: "Suporte" } } },
    { ok: true, status: 201, body: { status: "starting" } },
  ] });
  await malformed.send({ type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  const result = await malformed.send({ type: "CALL_START", number: "5511999999999" });
  assert.equal(result.ok, false);
  assert.match(result.error, /call ID/);
});
