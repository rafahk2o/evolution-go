(function (root, factory) {
  "use strict";

  if (typeof module === "object" && module.exports) {
    module.exports = factory(require("./shared/core.js"), require("./shared/protocol.js"));
    return;
  }
  root.importScripts("shared/core.js", "shared/protocol.js");
  root.WaCallsWorker = factory(root.WaCallsCore, root.WaCallsProtocol);
  root.WaCallsWorker.createWorker({
    chromeApi: root.chrome,
    fetchImpl: root.fetch.bind(root),
    cryptoApi: root.crypto,
    now: function () { return new Date(); },
  }).install();
})(typeof globalThis !== "undefined" ? globalThis : this, function (core, protocol) {
  "use strict";

  var CONFIG_KEY = "wacallsConfiguration";
  var CLIENT_KEY = "wacallsClientId";
  var WINDOW_KEY = "wacallsWindowId";
  var MAX_RESPONSE_BYTES = 1024 * 1024;

  function createWorker(dependencies) {
    var chromeApi = dependencies.chromeApi;
    var fetchImpl = dependencies.fetchImpl;
    var cryptoApi = dependencies.cryptoApi;
    var now = dependencies.now;

    function storageGet(key) {
      return chromeApi.storage.local.get(key).then(function (result) { return result[key]; });
    }

    function sanitizeConfig(config) {
      if (!config || !config.apiUrl || !config.apiKey) return { ok: true, configured: false };
      return {
        ok: true,
        configured: true,
        apiUrl: config.apiUrl,
        instanceName: config.instanceName || "",
        connected: config.connected === true,
        loggedIn: config.loggedIn === true,
        lastVerifiedAt: config.lastVerifiedAt || "",
      };
    }

    async function parseResponse(response) {
      var text = await response.text();
      if (text.length > MAX_RESPONSE_BYTES) throw new Error("A resposta da API excedeu o limite permitido.");
      if (!text) return {};
      try { return JSON.parse(text); }
      catch (_) { return { message: text.slice(0, 240) }; }
    }

    async function saveConfiguration(message) {
      var pattern = core.permissionPattern(message.apiUrl);
      var permitted = await chromeApi.permissions.contains({ origins: [pattern] });
      if (!permitted) throw new Error("Permissão para acessar a origem da API não foi concedida.");
      var response;
      var body;
      try {
        response = await fetchImpl(message.apiUrl + "/instance/status", {
          method: "GET",
          headers: { Accept: "application/json", apikey: message.apiKey },
        });
        body = await parseResponse(response);
      } catch (error) {
        throw new Error(core.callErrorMessage(0, {}, error));
      }
      if (!response.ok) throw new Error(core.callErrorMessage(response.status, body));
      var status = core.normalizeInstanceStatus(body);
      if (!status.connected || !status.loggedIn) {
        throw new Error("A instância precisa estar conectada e autenticada no WhatsApp.");
      }
      var config = {
        apiUrl: message.apiUrl,
        apiKey: message.apiKey,
        instanceName: status.instanceName,
        connected: status.connected,
        loggedIn: status.loggedIn,
        lastVerifiedAt: now().toISOString(),
      };
      await chromeApi.storage.local.set({ [CONFIG_KEY]: config });
      return sanitizeConfig(config);
    }

    async function ensureClientId() {
      var current = await storageGet(CLIENT_KEY);
      if (current) return current;
      var generated = "extension-" + cryptoApi.randomUUID();
      await chromeApi.storage.local.set({ [CLIENT_KEY]: generated });
      return generated;
    }

    async function evolutionRequest(config, clientId, path, options) {
      options = options || {};
      var headers = {
        Accept: "application/json",
        apikey: config.apiKey,
        "X-Call-Client-ID": clientId,
      };
      var init = { method: options.method || "GET", headers: headers };
      if (options.body !== undefined) {
        headers["Content-Type"] = "application/json";
        init.body = JSON.stringify(options.body);
      }
      if (options.keepalive) init.keepalive = true;
      var response;
      var body;
      try {
        response = await fetchImpl(config.apiUrl + path, init);
        body = await parseResponse(response);
      } catch (error) {
        throw new Error(core.callErrorMessage(0, {}, error));
      }
      if (!response.ok) {
        var upstream = new Error(core.callErrorMessage(response.status, body));
        upstream.status = response.status;
        throw upstream;
      }
      return body;
    }

    async function handleCall(message) {
      var config = await storageGet(CONFIG_KEY);
      if (!config || !config.apiUrl || !config.apiKey) {
        throw new Error("Configure e teste a Evolution GO antes de ligar.");
      }
      var clientId = await ensureClientId();
      if (message.type === protocol.TYPES.CALL_START) {
        var started = await evolutionRequest(config, clientId, "/call/start", {
          method: "POST",
          body: { number: message.number },
        });
        var startCallId = String(started.callId || "").trim();
        try { protocol.validateMessage({ type: protocol.TYPES.CALL_END, callId: startCallId }); }
        catch (_) { throw new Error("A Evolution GO retornou um call ID inválido."); }
        return {
          ok: true,
          callId: startCallId,
          direction: String(started.direction || ""),
          status: String(started.status || ""),
        };
      }
      if (message.type === protocol.TYPES.CALL_WEBRTC) {
        var negotiated = await evolutionRequest(config, clientId, "/call/" + encodeURIComponent(message.callId) + "/webrtc", {
          method: "POST",
          body: { sdpOffer: message.sdpOffer },
        });
        var answer = String(negotiated.sdpAnswer || "");
        if (!answer || answer.length > 262144) throw new Error("A Evolution GO retornou uma resposta SDP inválida.");
        return { ok: true, sdpAnswer: answer };
      }
      if (message.type === protocol.TYPES.CALL_ACTIVE) {
        var active = await evolutionRequest(config, clientId, "/call/active", { method: "GET" });
        var values = active.calls || active.data || [];
        if (!Array.isArray(values)) throw new Error("A Evolution GO retornou uma lista de chamadas inválida.");
        return { ok: true, calls: values.map(core.normalizeCall) };
      }
      if (message.type === protocol.TYPES.CALL_END) {
        var ended = await evolutionRequest(config, clientId, "/call/" + encodeURIComponent(message.callId), {
          method: "DELETE",
          keepalive: true,
        });
        return { ok: true, call: core.normalizeCall(ended) };
      }
      throw new Error("Operação interna não suportada.");
    }

    async function openCallWindow() {
      var windowId = await storageGet(WINDOW_KEY);
      if (windowId != null) {
        try {
          await chromeApi.windows.get(windowId);
          return chromeApi.windows.update(windowId, { focused: true });
        } catch (_) {
          await chromeApi.storage.local.remove(WINDOW_KEY);
        }
      }
      var created = await chromeApi.windows.create({
        url: "call-window.html",
        type: "popup",
        width: 380,
        height: 620,
        focused: true,
      });
      await chromeApi.storage.local.set({ [WINDOW_KEY]: created.id });
      return created;
    }

    async function handleMessage(raw) {
      try {
        var message = protocol.validateMessage(raw);
        if (message.type === protocol.TYPES.CONFIG_GET) return sanitizeConfig(await storageGet(CONFIG_KEY));
        if (message.type === protocol.TYPES.CONFIG_SAVE) return await saveConfiguration(message);
        return await handleCall(message);
      } catch (error) {
        var result = { ok: false, error: String(error && error.message || "Falha inesperada.") };
        if (error && error.status) result.status = error.status;
        return result;
      }
    }

    function initialize() {
      if (chromeApi.storage.local.setAccessLevel) {
        chromeApi.storage.local.setAccessLevel({ accessLevel: "TRUSTED_CONTEXTS" });
      }
      ensureClientId().catch(function () {});
    }

    function install() {
      initialize();
      chromeApi.action.onClicked.addListener(openCallWindow);
      chromeApi.windows.onRemoved.addListener(async function (removedId) {
        var storedId = await storageGet(WINDOW_KEY);
        if (storedId === removedId) await chromeApi.storage.local.remove(WINDOW_KEY);
      });
      chromeApi.runtime.onMessage.addListener(function (message, _sender, sendResponse) {
        handleMessage(message).then(sendResponse);
        return true;
      });
    }

    return {
      install: install,
      handleMessage: handleMessage,
      openCallWindow: openCallWindow,
      sanitizeConfig: sanitizeConfig,
    };
  }

  return { createWorker: createWorker };
});
