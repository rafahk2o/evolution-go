(function (root, factory) {
  "use strict";

  var api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
    return;
  }
  root.WaCallsWindow = api;
  root.document.addEventListener("DOMContentLoaded", function () {
    var app = api.createApp({
      document: root.document,
      window: root,
      chromeApi: root.chrome,
      core: root.WaCallsCore,
      protocol: root.WaCallsProtocol,
      controllerFactory: function (onState) {
        return root.WaCallsController.createController({
          sendMessage: function (message) { return root.chrome.runtime.sendMessage(message); },
          mediaDevices: root.navigator.mediaDevices,
          AudioContext: root.AudioContext || root.webkitAudioContext,
          AudioWorkletNode: root.AudioWorkletNode,
          RTCPeerConnection: root.RTCPeerConnection,
          runtimeGetURL: function (path) { return root.chrome.runtime.getURL(path); },
          setTimeout: root.setTimeout.bind(root),
          clearTimeout: root.clearTimeout.bind(root),
          setInterval: root.setInterval.bind(root),
          clearInterval: root.clearInterval.bind(root),
          now: Date.now,
          onState: onState,
        });
      },
      setInterval: root.setInterval.bind(root),
      clearInterval: root.clearInterval.bind(root),
      now: Date.now,
    });
    app.initialize();
  });
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  var statusLabels = {
    starting: "Preparando",
    ringing: "Chamando…",
    connected: "Conectada",
    ending: "Encerrando…",
    ended: "Encerrada",
    rejected: "Rejeitada",
    failed: "Falhou",
  };

  function createApp(deps) {
    var element = function (id) { return deps.document.getElementById(id); };
    var action = function (name) { return deps.document.querySelector('[data-action="' + name + '"]'); };
    var elements = {
      setup: element("setupPanel"), dialer: element("dialerPanel"), active: element("activePanel"),
      apiUrl: element("apiUrl"), apiKey: element("apiKey"), phone: element("phone"),
      instanceName: element("instanceName"), callNumber: element("callNumber"), callStatus: element("callStatus"),
      duration: element("duration"), muteLabel: element("muteLabel"), message: element("message"), save: action("save"), settings: action("settings"),
      call: action("call"), mute: action("mute"), end: action("end"),
    };
    var configured = false;
    var editing = false;
    var config = null;
    var controller = null;
    var callState = { phase: "idle", number: "", status: "", connectedAt: 0, muted: false, busy: false, error: "" };
    var durationTimer = null;
    var bound = false;

    function isActive() {
      return callState.phase === "preparing" || callState.phase === "active" || callState.phase === "ending";
    }

    function formatDuration() {
      if (!callState.connectedAt) return "00:00";
      var seconds = Math.max(0, Math.floor((deps.now() - callState.connectedAt) / 1000));
      var minutes = Math.floor(seconds / 60);
      return String(minutes).padStart(2, "0") + ":" + String(seconds % 60).padStart(2, "0");
    }

    function setMessage(message, error) {
      elements.message.textContent = message || "";
      elements.message.classList.toggle("error", !!message && !!error);
      elements.message.classList.toggle("success", !!message && !error);
    }

    function render() {
      var active = isActive();
      elements.setup.hidden = configured && !editing;
      elements.dialer.hidden = !configured || editing || active;
      elements.active.hidden = !configured || !active;
      elements.settings.disabled = active;
      elements.call.disabled = callState.busy;
      elements.end.disabled = callState.busy && callState.phase !== "active";
      elements.mute.disabled = callState.phase !== "active";
      elements.phone.disabled = active;
      if (config) {
        elements.apiUrl.value = editing ? config.apiUrl : elements.apiUrl.value;
        elements.instanceName.textContent = config.instanceName || "Instância conectada";
      }
      elements.callNumber.textContent = callState.number || "";
      elements.callStatus.textContent = statusLabels[callState.status] || statusLabels[callState.phase] || "Preparando";
      elements.duration.textContent = formatDuration();
      elements.muteLabel.textContent = callState.muted ? "Ativar microfone" : "Silenciar";
      if (callState.error) setMessage(callState.error, true);
    }

    function ensureController() {
      if (!controller && configured) {
        controller = deps.controllerFactory(function (value) {
          callState = Object.assign({}, callState, value);
          render();
        });
      }
      return controller;
    }

    async function saveConfiguration(event) {
      if (event && event.preventDefault) event.preventDefault();
      var apiKey = elements.apiKey.value;
      elements.apiKey.value = "";
      try {
        var origin = deps.core.normalizeApiOrigin(elements.apiUrl.value);
        var granted = await deps.chromeApi.permissions.request({ origins: [deps.core.permissionPattern(origin)] });
        if (!granted) throw new Error("Permissão para acessar a API não foi concedida.");
        var result = await deps.chromeApi.runtime.sendMessage({
          type: deps.protocol.TYPES.CONFIG_SAVE,
          apiUrl: origin,
          apiKey: apiKey,
        });
        if (!result || !result.ok) throw new Error(result && result.error || "Não foi possível salvar a configuração.");
        configured = true;
        editing = false;
        config = result;
        ensureController();
        setMessage("Instância conectada com sucesso.", false);
      } catch (error) {
        setMessage(error.message, true);
      }
      render();
    }

    async function startCall() {
      try { await ensureController().start(elements.phone.value); }
      catch (error) { setMessage(error.message, true); }
    }
    function toggleMute() { if (controller) controller.toggleMute(); }
    async function endCall() {
      if (!controller) return;
      try { await controller.end(); }
      catch (error) { setMessage(error.message, true); }
    }

    function bind() {
      if (bound) return;
      bound = true;
      elements.save.addEventListener("click", saveConfiguration);
      elements.call.addEventListener("click", startCall);
      elements.mute.addEventListener("click", toggleMute);
      elements.end.addEventListener("click", endCall);
      elements.settings.addEventListener("click", function () { if (!isActive()) { editing = true; render(); } });
      deps.window.addEventListener("pagehide", function () {
        if (controller) return controller.dispose({ endRemote: true });
      });
    }

    async function initialize() {
      bind();
      var result = await deps.chromeApi.runtime.sendMessage({ type: deps.protocol.TYPES.CONFIG_GET });
      configured = !!(result && result.ok && result.configured);
      config = configured ? result : null;
      elements.apiKey.value = "";
      if (config) elements.apiUrl.value = config.apiUrl;
      ensureController();
      if (!durationTimer) durationTimer = deps.setInterval(function () { if (isActive()) elements.duration.textContent = formatDuration(); }, 1000);
      render();
    }

    return { initialize: initialize, render: render, saveConfiguration: saveConfiguration };
  }

  return { createApp: createApp };
});
