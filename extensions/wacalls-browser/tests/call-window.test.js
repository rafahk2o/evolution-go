"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const windowModule = require("../call-window.js");
const core = require("../shared/core.js");
const protocol = require("../shared/protocol.js");

const root = path.join(__dirname, "..");

test("contains accessible setup, dialer, and active-call controls", () => {
  const html = fs.readFileSync(path.join(root, "call-window.html"), "utf8");
  ["apiUrl", "apiKey", "phone"].forEach((id) => {
    assert.match(html, new RegExp('<label[^>]+for="' + id + '"'));
    assert.match(html, new RegExp('id="' + id + '"'));
  });
  ["save", "settings", "call", "mute", "end", "download-recording"].forEach((action) => {
    assert.match(html, new RegExp('data-action="' + action + '"'));
  });
  assert.match(html, /role="status"/);
  assert.match(html, /id="muteLabel"/);
  assert.match(html, /id="recordingIndicator"/);
  assert.match(html, /id="recordingPanel"/);
  assert.match(html, /gravadas automaticamente/i);
  assert.match(html, /neste navegador/i);
  assert.doesNotMatch(html, /<script[^>]*>\s*[^<\s]/);
});

function element() {
  const listeners = {};
  return {
    hidden: false,
    disabled: false,
    value: "",
    textContent: "",
    dataset: {},
    classList: { values: new Set(), toggle(name, enabled) { if (enabled) this.values.add(name); else this.values.delete(name); } },
    addEventListener(type, listener) { listeners[type] = listener; },
    async emit(type, value) { if (listeners[type]) return listeners[type](value || { preventDefault() {} }); },
  };
}

function createHarness(options) {
  options = options || {};
  const ids = ["setupPanel", "dialerPanel", "activePanel", "apiUrl", "apiKey", "phone", "instanceName", "callNumber", "callStatus", "duration", "muteLabel", "message", "recordingIndicator", "recordingPanel", "recordingSize"];
  const actions = ["save", "settings", "call", "mute", "end", "download-recording"];
  const elements = {};
  ids.forEach((id) => { elements[id] = element(); });
  actions.forEach((action) => { elements["action:" + action] = element(); });
  const windowListeners = {};
  const messages = [];
  const permissionRequests = [];
  const controllerCalls = [];
  const downloads = [];
  const revokedUrls = [];
  let onControllerState = null;
  const config = options.config || { ok: true, configured: false };
  const chromeApi = {
    permissions: { async request(value) { permissionRequests.push(value); return options.permission !== false; } },
    runtime: {
      async sendMessage(message) {
        messages.push(message);
        if (message.type === "CONFIG_GET") return config;
        return { ok: true, configured: true, apiUrl: message.apiUrl, instanceName: "Suporte", connected: true, loggedIn: true };
      },
    },
  };
  const app = windowModule.createApp({
    document: {
      getElementById(id) { return elements[id]; },
      querySelector(selector) { return elements["action:" + selector.match(/data-action="([^"]+)/)[1]]; },
      createElement(name) {
        assert.equal(name, "a");
        return {
          href: "", download: "",
          click() { downloads.push({ href: this.href, download: this.download }); },
          remove() {},
        };
      },
      body: { appendChild() {} },
    },
    window: { addEventListener(type, listener) { windowListeners[type] = listener; } },
    chromeApi,
    core,
    protocol,
    controllerFactory(onState) {
      onControllerState = onState;
      return {
        async start(number) { controllerCalls.push(["start", number]); },
        toggleMute() { controllerCalls.push(["mute"]); return true; },
        async end() { controllerCalls.push(["end"]); },
        async dispose(value) { controllerCalls.push(["dispose", value]); },
        getRecording() { return options.recording || null; },
      };
    },
    setInterval() { return 1; },
    clearInterval() {},
    setTimeout(fn) { fn(); return 1; },
    urlApi: {
      createObjectURL(blob) { assert.equal(blob, options.recording.blob); return "blob:recording"; },
      revokeObjectURL(url) { revokedUrls.push(url); },
    },
    now: () => 61000,
  });
  return { app, elements, messages, permissionRequests, controllerCalls, windowListeners, downloads, revokedUrls, emitState(value) { onControllerState(value); } };
}

test("shows setup when unconfigured and dialer when configured", async () => {
  const unconfigured = createHarness();
  await unconfigured.app.initialize();
  assert.equal(unconfigured.elements.setupPanel.hidden, false);
  assert.equal(unconfigured.elements.dialerPanel.hidden, true);

  const configured = createHarness({ config: { ok: true, configured: true, apiUrl: "https://api.example", instanceName: "Suporte", connected: true, loggedIn: true } });
  await configured.app.initialize();
  assert.equal(configured.elements.setupPanel.hidden, true);
  assert.equal(configured.elements.dialerPanel.hidden, false);
  assert.equal(configured.elements.apiKey.value, "");
});

test("requests exact permission, submits the key once, and clears the field", async () => {
  const harness = createHarness();
  await harness.app.initialize();
  harness.elements.apiUrl.value = "https://api.example/";
  harness.elements.apiKey.value = "secret";
  await harness.elements["action:save"].emit("click");
  assert.deepEqual(harness.permissionRequests[0], { origins: ["https://api.example/*"] });
  assert.deepEqual(harness.messages[1], { type: "CONFIG_SAVE", apiUrl: "https://api.example", apiKey: "secret" });
  assert.equal(harness.elements.apiKey.value, "");
});

test("does not send the key when permission is denied", async () => {
  const harness = createHarness({ permission: false });
  await harness.app.initialize();
  harness.elements.apiUrl.value = "https://api.example";
  harness.elements.apiKey.value = "secret";
  await harness.elements["action:save"].emit("click");
  assert.equal(harness.messages.length, 1);
  assert.equal(harness.elements.apiKey.value, "");
});

test("routes call controls and disposes on pagehide", async () => {
  const harness = createHarness({ config: { ok: true, configured: true, apiUrl: "https://api.example", instanceName: "Suporte", connected: true, loggedIn: true } });
  await harness.app.initialize();
  harness.elements.phone.value = "+55 11 99999-9999";
  await harness.elements["action:call"].emit("click");
  await harness.elements["action:mute"].emit("click");
  await harness.elements["action:end"].emit("click");
  await harness.windowListeners.pagehide();
  assert.deepEqual(harness.controllerCalls, [
    ["start", "+55 11 99999-9999"],
    ["mute"],
    ["end"],
    ["dispose", { endRemote: true }],
  ]);
});

test("renders active status, duration, and locks settings", async () => {
  const harness = createHarness({ config: { ok: true, configured: true, apiUrl: "https://api.example", instanceName: "Suporte", connected: true, loggedIn: true } });
  await harness.app.initialize();
  harness.emitState({ phase: "active", number: "5511999999999", status: "connected", connectedAt: 1000, muted: false, busy: false, error: "" });
  assert.equal(harness.elements.activePanel.hidden, false);
  assert.equal(harness.elements.callStatus.textContent, "Conectada");
  assert.equal(harness.elements.duration.textContent, "01:00");
  assert.equal(harness.elements["action:settings"].disabled, true);
  assert.equal(harness.elements.muteLabel.textContent, "Silenciar");
});

test("renders automatic recording status and the completed recording", async () => {
  const harness = createHarness({ config: { ok: true, configured: true, apiUrl: "https://api.example", instanceName: "Suporte" } });
  await harness.app.initialize();
  harness.emitState({ phase: "active", recordingStatus: "recording", recordingAvailable: false });
  assert.equal(harness.elements.recordingIndicator.hidden, false);
  assert.equal(harness.elements.recordingPanel.hidden, true);

  harness.emitState({ phase: "idle", recordingStatus: "ready", recordingAvailable: true, recordingBytes: 1536 });
  assert.equal(harness.elements.recordingIndicator.hidden, true);
  assert.equal(harness.elements.recordingPanel.hidden, false);
  assert.equal(harness.elements.recordingSize.textContent, "1,5 KB");
});

test("downloads the completed recording only when requested", async () => {
  const recording = { blob: new Blob(["audio"]), filename: "wacalls-2026.webm" };
  const harness = createHarness({
    config: { ok: true, configured: true, apiUrl: "https://api.example", instanceName: "Suporte" },
    recording,
  });
  await harness.app.initialize();
  harness.emitState({ phase: "idle", recordingStatus: "ready", recordingAvailable: true, recordingBytes: recording.blob.size });
  assert.equal(harness.downloads.length, 0);
  await harness.elements["action:download-recording"].emit("click");
  assert.deepEqual(harness.downloads, [{ href: "blob:recording", download: "wacalls-2026.webm" }]);
  assert.deepEqual(harness.revokedUrls, ["blob:recording"]);
});

test("recording failures warn without disabling call controls and a new call hides old results", async () => {
  const harness = createHarness({ config: { ok: true, configured: true, apiUrl: "https://api.example", instanceName: "Suporte" } });
  await harness.app.initialize();
  harness.emitState({ phase: "active", recordingStatus: "failed", recordingAvailable: false, recordingError: "Falha ao gravar a chamada." });
  assert.equal(harness.elements.activePanel.hidden, false);
  assert.equal(harness.elements["action:mute"].disabled, false);
  assert.equal(harness.elements.message.textContent, "Falha ao gravar a chamada.");

  harness.emitState({ phase: "idle", recordingStatus: "ready", recordingAvailable: true, recordingBytes: 3 });
  assert.equal(harness.elements.recordingPanel.hidden, false);
  harness.emitState({ phase: "preparing", recordingStatus: "inactive", recordingAvailable: false, recordingBytes: 0, recordingError: "" });
  assert.equal(harness.elements.recordingPanel.hidden, true);
});
