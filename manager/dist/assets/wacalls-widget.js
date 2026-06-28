(function () {
  "use strict";

  var core = window.WaCallsWidgetCore;
  if (!core || !window.fetch || !window.RTCPeerConnection) {
    console.warn("[WaCalls widget] APIs obrigatórias indisponíveis.");
    return;
  }

  var CLIENT_STORAGE_KEY = "evolution-wacalls-client-id";
  var ROOT_ID = "wacalls-modal-root";
  var TERMINAL_RETENTION_MS = 4000;
  var STREAM_RECONNECT_MS = 2500;
  var state = {
    instances: [],
    calls: new Map(),
    streams: new Map(),
    currentInstanceId: "",
    media: null,
    loadingInstances: false,
    busy: false,
    muted: false,
    message: "",
    messageIsError: false,
    modalTimer: null,
  };

  var css = ""
    + ".wc-action-bar{display:flex!important;align-items:stretch!important;overflow:hidden!important}"
    + ".wc-action-bar>button{padding-left:7px!important;padding-right:7px!important}"
    + ".wc-action-bar>button.flex-1{flex:1 1 116px!important;min-width:116px!important;max-width:none!important;overflow:hidden!important;white-space:nowrap!important;text-overflow:ellipsis!important;padding-left:8px!important;padding-right:8px!important}"
    + ".wc-action-bar>button:not(.flex-1){flex:0 0 38px!important;width:38px!important;padding-left:0!important;padding-right:0!important}"
    + ".wc-action-button{position:relative;display:inline-flex;align-items:center;justify-content:center;width:38px;height:48px;border:0;background:transparent;color:#00e995;cursor:pointer;flex:0 0 38px!important;}"
    + ".wc-action-button:hover{background:rgba(0,233,149,.12)}"
    + ".wc-action-button:disabled{opacity:.35;cursor:not-allowed}"
    + ".wc-action-button svg{width:20px;height:20px;fill:none;stroke:currentColor;stroke-width:2;stroke-linecap:round;stroke-linejoin:round}"
    + ".wc-badge{position:absolute;right:6px;top:6px;width:10px;height:10px;border-radius:999px;background:#ef4444;box-shadow:0 0 0 3px rgba(239,68,68,.2);animation:wc-pulse 1.2s infinite}"
    + "@keyframes wc-pulse{0%,100%{transform:scale(1);opacity:1}50%{transform:scale(1.35);opacity:.65}}"
    + ".wc-overlay{position:fixed;inset:0;z-index:10020;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.72);padding:16px}"
    + ".wc-modal{width:min(520px,100%);max-height:94vh;overflow:auto;background:var(--background,#0b0b0b);border:1px solid var(--border,#2a2a2a);border-radius:12px;color:var(--foreground,#f5f5f5);box-shadow:0 24px 80px rgba(0,0,0,.55);font-family:Inter,ui-sans-serif,system-ui,sans-serif}"
    + ".wc-header{display:flex;align-items:center;justify-content:space-between;padding:16px 18px;border-bottom:1px solid var(--border,#2a2a2a)}"
    + ".wc-title{display:flex;align-items:center;gap:10px;font-size:16px;font-weight:700}"
    + ".wc-title svg{width:20px;height:20px;stroke:#00e995;fill:none;stroke-width:2}"
    + ".wc-close{border:0;background:transparent;color:#9ca3af;font-size:25px;cursor:pointer}"
    + ".wc-body{padding:18px}"
    + ".wc-status{display:flex;align-items:center;justify-content:space-between;padding:12px;border:1px solid var(--border,#2a2a2a);border-radius:9px;background:rgba(255,255,255,.025);margin-bottom:16px}"
    + ".wc-status-main{display:flex;flex-direction:column;gap:4px}"
    + ".wc-status-label{font-size:13px;font-weight:700;color:#00e995}"
    + ".wc-status-peer{font-size:12px;color:#a1a1aa}"
    + ".wc-duration{font:600 18px ui-monospace,SFMono-Regular,monospace}"
    + ".wc-field{display:flex;flex-direction:column;gap:7px;margin-bottom:14px}"
    + ".wc-field label{font-size:12px;font-weight:600;color:#d4d4d8}"
    + ".wc-field input{height:42px;border:1px solid var(--input,#333);border-radius:8px;background:#111;color:#fff;padding:0 12px;font-size:14px;outline:none}"
    + ".wc-field input:focus{border-color:#00e995;box-shadow:0 0 0 2px rgba(0,233,149,.14)}"
    + ".wc-alert{padding:10px 12px;border-radius:8px;margin-bottom:14px;font-size:12px;line-height:1.4}"
    + ".wc-alert.error{background:rgba(239,68,68,.12);border:1px solid rgba(239,68,68,.35);color:#fecaca}"
    + ".wc-alert.ok{background:rgba(0,233,149,.1);border:1px solid rgba(0,233,149,.3);color:#b6ffe6}"
    + ".wc-actions{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}"
    + ".wc-button{height:42px;border:1px solid transparent;border-radius:8px;padding:0 14px;font-weight:700;font-size:13px;cursor:pointer}"
    + ".wc-button:disabled{opacity:.5;cursor:wait}"
    + ".wc-primary{background:#00e995;color:#002d1e}"
    + ".wc-secondary{background:#202020;border-color:#383838;color:#f4f4f5}"
    + ".wc-danger{background:#dc2626;color:#fff}"
    + ".wc-warning{background:#f59e0b;color:#1c1200}"
    + ".wc-wide{grid-column:1/-1}"
    + ".wc-hint{font-size:11px;line-height:1.45;color:#8b8b94;margin-top:12px}"
    + ".wc-spinner{display:inline-block;width:13px;height:13px;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;animation:wc-spin .8s linear infinite;vertical-align:-2px;margin-right:7px}"
    + "@keyframes wc-spin{to{transform:rotate(360deg)}}";

  var style = document.createElement("style");
  style.textContent = css;
  document.head.appendChild(style);

  function authState() {
    try {
      var parsed = JSON.parse(localStorage.getItem("evolution-auth") || "{}");
      var value = parsed.state || parsed;
      return {
        apiUrl: String(value.apiUrl || window.location.origin).replace(/\/$/, ""),
        apiKey: String(value.apiKey || ""),
      };
    } catch (_) {
      return { apiUrl: window.location.origin, apiKey: "" };
    }
  }

  function clientId() {
    var current = localStorage.getItem(CLIENT_STORAGE_KEY);
    if (current) return current;
    var generated = "manager-" + (window.crypto && window.crypto.randomUUID
      ? window.crypto.randomUUID()
      : Date.now().toString(36) + "-" + Math.random().toString(36).slice(2));
    localStorage.setItem(CLIENT_STORAGE_KEY, generated);
    return generated;
  }

  function normalizeInstance(item) {
    return {
      id: String(item.id || item.instanceId || ""),
      name: String(item.name || item.instanceName || ""),
      token: String(item.token || item.apikey || item.apiKey || ""),
      connected: item.connected === true || item.status === "open",
    };
  }

  function getInstance(id) {
    return state.instances.find(function (instance) { return instance.id === id; }) || null;
  }

  function esc(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function parseBody(text) {
    if (!text) return {};
    try { return JSON.parse(text); } catch (_) { return { message: text }; }
  }

  function request(instance, path, options) {
    options = options || {};
    var auth = authState();
    var headers = Object.assign({
      Accept: "application/json",
      apikey: instance.token,
      "X-Call-Client-ID": clientId(),
    }, options.headers || {});
    var init = { method: options.method || "GET", headers: headers };
    if (options.body !== undefined) {
      headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(options.body);
    }
    if (options.signal) init.signal = options.signal;
    if (options.keepalive) init.keepalive = true;
    return fetch(auth.apiUrl + path, init).then(function (response) {
      return response.text().then(function (text) {
        var body = parseBody(text);
        if (!response.ok) {
          var error = new Error(core.callErrorMessage(response.status, body));
          error.status = response.status;
          error.body = body;
          throw error;
        }
        return body;
      });
    });
  }

  function root() {
    var element = document.getElementById(ROOT_ID);
    if (!element) {
      element = document.createElement("div");
      element.id = ROOT_ID;
      document.body.appendChild(element);
    }
    return element;
  }

  function findCard(instance) {
    var nodes = Array.prototype.slice.call(document.querySelectorAll("main div, body div"));
    var matches = nodes.filter(function (node) {
      if (node.id === ROOT_ID || node.closest("#" + ROOT_ID)) return false;
      var rectangle = node.getBoundingClientRect();
      if (rectangle.width < 180 || rectangle.height < 120 || rectangle.width > 680 || rectangle.height > 540) return false;
      var text = node.innerText || "";
      return core.matchesInstanceName(text, instance.name) && text.indexOf("Status") !== -1;
    });
    matches.sort(function (left, right) {
      var a = left.getBoundingClientRect();
      var b = right.getBoundingClientRect();
      return (a.width * a.height) - (b.width * b.height);
    });
    return matches[0] || null;
  }

  function actionRow(card) {
    var buttons = Array.prototype.slice.call(card.querySelectorAll("button"));
    var rows = buttons.map(function (button) { return button.parentElement; }).filter(Boolean);
    rows.sort(function (left, right) {
      return right.querySelectorAll("button").length - left.querySelectorAll("button").length;
    });
    return rows[0] || null;
  }

  function phoneIcon() {
    return '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6A19.79 19.79 0 0 1 2.12 4.18 2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.12.9.33 1.78.62 2.63a2 2 0 0 1-.45 2.11L8 9.73a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.85.29 1.73.5 2.63.62A2 2 0 0 1 22 16.92z"/></svg>';
  }

  function hasIncoming(call) {
    return call && call.direction === "incoming" && call.status === "offered";
  }

  function updateButton(instance, button) {
    button.disabled = !instance.connected;
    button.title = instance.connected ? "Testar chamadas" : "Conecte a instância para testar chamadas";
    var incoming = hasIncoming(state.calls.get(instance.id));
    var badge = button.querySelector(".wc-badge");
    if (incoming && !badge) {
      badge = document.createElement("span");
      badge.className = "wc-badge";
      badge.title = "Chamada recebida";
      button.appendChild(badge);
    } else if (!incoming && badge) {
      badge.remove();
    }
  }

  function injectButtons() {
    if (!/^\/manager(\/|$)/.test(window.location.pathname)) return;
    state.instances.forEach(function (instance) {
      var card = findCard(instance);
      if (!card) return;
      var actions = actionRow(card);
      if (!actions) return;
      actions.classList.add("wc-action-bar");
      var selector = '[data-wacalls-button-for="' + instance.id + '"]';
      var button = actions.querySelector(selector);
      if (!button) {
        button = document.createElement("button");
        button.type = "button";
        button.className = "wc-action-button";
        button.setAttribute("data-wacalls-button-for", instance.id);
        button.innerHTML = phoneIcon();
        button.addEventListener("click", function (event) {
          event.preventDefault();
          event.stopPropagation();
          if (!button.disabled) openModal(instance.id);
        });
        var gear = Array.prototype.slice.call(actions.querySelectorAll("button")).find(function (candidate) {
          var classes = candidate.className || "";
          return classes.indexOf("text-gray-500") !== -1 && classes.indexOf("text-red-500") === -1;
        });
        if (gear) actions.insertBefore(button, gear);
        else actions.appendChild(button);
      }
      updateButton(instance, button);
    });
  }

  function callStartedAt(call) {
    if (!call) return Date.now();
    if (call._startedAt) return call._startedAt;
    var parsed = Date.parse(call.createdAt || call.timestamp || "");
    call._startedAt = Number.isFinite(parsed) ? parsed : Date.now();
    return call._startedAt;
  }

  function mergeCall(instanceId, value) {
    var next = core.normalizeCall(value);
    if (!next.instanceId) next.instanceId = instanceId;
    var previous = state.calls.get(instanceId);
    next._startedAt = previous ? previous._startedAt : callStartedAt(value || next);
    next.reason = String((value && value.reason) || "");
    state.calls.set(instanceId, next);
    updateCallUI(instanceId);
    if (core.isTerminalStatus(next.status)) {
      if (state.media && state.media.callId === next.callId) cleanupMedia();
      window.setTimeout(function () {
        var current = state.calls.get(instanceId);
        if (current && current.callId === next.callId && core.isTerminalStatus(current.status)) {
          state.calls.delete(instanceId);
          updateCallUI(instanceId);
        }
      }, TERMINAL_RETENTION_MS);
    }
    return next;
  }

  function updateCallUI(instanceId) {
    var instance = getInstance(instanceId);
    if (instance) {
      var button = document.querySelector('[data-wacalls-button-for="' + instance.id + '"]');
      if (button) updateButton(instance, button);
    }
    if (state.currentInstanceId === instanceId && root().innerHTML) renderModal();
  }

  function handleCallEvent(instance, event) {
    if (!event || !event.data || typeof event.data !== "object") return;
    mergeCall(instance.id, event.data);
  }

  function reconcile(instance) {
    return request(instance, "/call/active").then(function (body) {
      var calls = body.calls || body.data || [];
      if (Array.isArray(calls) && calls.length) {
        calls.forEach(function (call) { mergeCall(instance.id, call); });
      } else {
        var previous = state.calls.get(instance.id);
        if (previous && !core.isTerminalStatus(previous.status)) {
          state.calls.delete(instance.id);
          if (state.media && state.media.callId === previous.callId) cleanupMedia();
          updateCallUI(instance.id);
        }
      }
      return calls;
    });
  }

  async function consumeEventStream(instance, entry) {
    while (!entry.controller.signal.aborted && getInstance(instance.id)) {
      try {
        var auth = authState();
        var response = await fetch(auth.apiUrl + "/call/events", {
          headers: {
            Accept: "text/event-stream",
            apikey: instance.token,
            "X-Call-Client-ID": clientId(),
          },
          signal: entry.controller.signal,
        });
        if (!response.ok) {
          var text = await response.text();
          throw new Error(core.callErrorMessage(response.status, parseBody(text)));
        }
        if (!response.body) throw new Error("Stream de eventos indisponível.");
        reconcile(instance).catch(function () {});
        var parser = core.createSSEParser(function (event) { handleCallEvent(instance, event); });
        var reader = response.body.getReader();
        var decoder = new TextDecoder();
        while (!entry.controller.signal.aborted) {
          var chunk = await reader.read();
          if (chunk.done) break;
          parser.push(decoder.decode(chunk.value, { stream: true }));
        }
        parser.push(decoder.decode());
        parser.end();
      } catch (error) {
        if (entry.controller.signal.aborted) return;
        console.warn("[WaCalls widget] stream interrompido para", instance.name, error.message);
      }
      await new Promise(function (resolve) { window.setTimeout(resolve, STREAM_RECONNECT_MS); });
    }
  }

  function syncStreams() {
    var valid = new Map();
    state.instances.forEach(function (instance) {
      if (!instance.connected || !instance.token) return;
      valid.set(instance.id, instance);
      var existing = state.streams.get(instance.id);
      if (existing && existing.token === instance.token) return;
      if (existing) existing.controller.abort();
      var entry = { token: instance.token, controller: new AbortController() };
      state.streams.set(instance.id, entry);
      consumeEventStream(instance, entry);
    });
    state.streams.forEach(function (entry, instanceId) {
      if (!valid.has(instanceId)) {
        entry.controller.abort();
        state.streams.delete(instanceId);
      }
    });
  }

  function loadInstances() {
    if (state.loadingInstances) return;
    var auth = authState();
    if (!auth.apiKey) return;
    state.loadingInstances = true;
    fetch(auth.apiUrl + "/instance/all", { headers: { apikey: auth.apiKey, Accept: "application/json" } })
      .then(function (response) {
        return response.text().then(function (text) {
          var body = parseBody(text);
          if (!response.ok) throw new Error(core.callErrorMessage(response.status, body));
          return body;
        });
      })
      .then(function (body) {
        var list = Array.isArray(body) ? body : body.data || body.instances || [];
        state.instances = list.map(normalizeInstance).filter(function (instance) {
          return instance.id && instance.name && instance.token;
        });
        syncStreams();
        injectButtons();
      })
      .catch(function (error) {
        console.warn("[WaCalls widget] falha ao carregar instâncias:", error.message);
      })
      .finally(function () { state.loadingInstances = false; });
  }

  function statusLabel(status) {
    var labels = {
      offered: "Chamada recebida",
      starting: "Preparando chamada",
      ringing: "Chamando",
      connected: "Em chamada",
      ending: "Encerrando",
      ended: "Chamada encerrada",
      rejected: "Chamada rejeitada",
      failed: "Falha na chamada",
    };
    return labels[status] || "Pronto para ligar";
  }

  function formatDuration(milliseconds) {
    var total = Math.max(0, Math.floor(milliseconds / 1000));
    var minutes = Math.floor(total / 60).toString().padStart(2, "0");
    var seconds = (total % 60).toString().padStart(2, "0");
    return minutes + ":" + seconds;
  }

  function updateDuration() {
    var output = root().querySelector("[data-wc-duration]");
    var call = state.calls.get(state.currentInstanceId);
    if (output && call) output.textContent = formatDuration(Date.now() - callStartedAt(call));
  }

  function renderModal(message, isError) {
    if (message !== undefined) {
      state.message = String(message || "");
      state.messageIsError = !!isError;
    }
    var instance = getInstance(state.currentInstanceId);
    if (!instance) {
      forceCloseModal();
      return;
    }
    var call = state.calls.get(instance.id);
    var active = call && !core.isTerminalStatus(call.status);
    var incoming = hasIncoming(call);
    var status = call ? statusLabel(call.status) : "Pronto para ligar";
    var peer = call && call.peer ? call.peer : instance.name;
    var actions = "";

    if (!call || core.isTerminalStatus(call.status)) {
      actions = '<button class="wc-button wc-primary wc-wide" type="button" data-wc-action="start">Ligar</button>';
    } else if (incoming) {
      actions = '<button class="wc-button wc-primary" type="button" data-wc-action="accept">Aceitar</button>'
        + '<button class="wc-button wc-danger" type="button" data-wc-action="reject">Rejeitar</button>';
    } else {
      actions = '<button class="wc-button wc-secondary" type="button" data-wc-action="mute">'
        + (state.muted ? "Ativar microfone" : "Silenciar") + '</button>'
        + '<button class="wc-button wc-danger" type="button" data-wc-action="end">Encerrar</button>';
    }

    root().innerHTML = ''
      + '<div class="wc-overlay">'
      + '<div class="wc-modal" role="dialog" aria-modal="true" aria-label="Teste de chamadas">'
      + '<div class="wc-header"><div class="wc-title">' + phoneIcon() + '<span>Chamadas — ' + esc(instance.name) + '</span></div><button class="wc-close" type="button" data-wc-close>&times;</button></div>'
      + '<div class="wc-body">'
      + (state.message ? '<div class="wc-alert ' + (state.messageIsError ? "error" : "ok") + '">' + esc(state.message) + '</div>' : '')
      + '<div class="wc-status"><div class="wc-status-main"><span class="wc-status-label">' + (state.busy ? '<span class="wc-spinner"></span>' : '') + esc(status) + '</span><span class="wc-status-peer">' + esc(peer) + '</span></div>'
      + (call ? '<span class="wc-duration" data-wc-duration>' + formatDuration(Date.now() - callStartedAt(call)) + '</span>' : '') + '</div>'
      + (!active ? '<div class="wc-field"><label>Número com DDI e DDD</label><input type="tel" inputmode="numeric" data-wc-number placeholder="5511999999999" autocomplete="tel"></div>' : '')
      + '<div class="wc-actions">' + actions + '</div>'
      + '<p class="wc-hint">O navegador solicitará acesso ao microfone. Para áudio fora de localhost, abra o Manager por HTTPS.</p>'
      + '</div></div></div>';

    Array.prototype.slice.call(root().querySelectorAll("button")).forEach(function (button) {
      if (state.busy && !button.hasAttribute("data-wc-close")) button.disabled = true;
    });
    root().querySelector("[data-wc-close]").addEventListener("click", closeModal);
    var action = root().querySelector("[data-wc-action]");
    Array.prototype.slice.call(root().querySelectorAll("[data-wc-action]")).forEach(function (button) {
      button.addEventListener("click", function () {
        var name = button.getAttribute("data-wc-action");
        if (name === "start") startCall();
        if (name === "accept") acceptCall();
        if (name === "reject") rejectCall(false);
        if (name === "end") endCall(false);
        if (name === "mute") toggleMute();
      });
    });
    if (action && action.getAttribute("data-wc-action") === "start") {
      var input = root().querySelector("[data-wc-number]");
      if (input) input.focus();
    }
    window.clearInterval(state.modalTimer);
    state.modalTimer = window.setInterval(updateDuration, 1000);
  }

  function openModal(instanceId) {
    state.currentInstanceId = instanceId;
    state.message = "";
    state.messageIsError = false;
    renderModal();
    var instance = getInstance(instanceId);
    if (instance) reconcile(instance).catch(function (error) { renderModal(error.message, true); });
  }

  async function closeModal() {
    var instance = getInstance(state.currentInstanceId);
    var call = state.calls.get(state.currentInstanceId);
    if (instance && call && !core.isTerminalStatus(call.status)) {
      if (!window.confirm("Existe uma chamada em andamento. Deseja encerrá-la?")) return;
      if (hasIncoming(call)) await rejectCall(true);
      else await endCall(true);
      return;
    }
    forceCloseModal();
  }

  function forceCloseModal() {
    window.clearInterval(state.modalTimer);
    state.modalTimer = null;
    state.currentInstanceId = "";
    state.message = "";
    root().innerHTML = "";
  }

  function setBusy(value, message) {
    state.busy = value;
    if (message !== undefined) {
      state.message = message;
      state.messageIsError = false;
    }
    if (state.currentInstanceId) renderModal();
  }

  async function setupAudio() {
    var stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false });
    var Context = window.AudioContext || window.webkitAudioContext;
    if (!Context || typeof AudioWorkletNode === "undefined") {
      stream.getTracks().forEach(function (track) { track.stop(); });
      throw new Error("AudioWorklet não é suportado neste navegador.");
    }
    var context = new Context({ latencyHint: "interactive" });
    try {
      await context.audioWorklet.addModule("/assets/wacalls-audio-worklet.js");
      await context.resume();
      var source = context.createMediaStreamSource(stream);
      var capture = new AudioWorkletNode(context, "wacalls-capture");
      var playback = new AudioWorkletNode(context, "wacalls-playback", { outputChannelCount: [1] });
      var silent = context.createGain();
      silent.gain.value = 0;
      source.connect(capture);
      capture.connect(silent);
      silent.connect(context.destination);
      playback.connect(context.destination);
      return { stream: stream, context: context, source: source, capture: capture, playback: playback, silent: silent };
    } catch (error) {
      stream.getTracks().forEach(function (track) { track.stop(); });
      try { await context.close(); } catch (_) {}
      throw error;
    }
  }

  function waitForICE(peer) {
    if (peer.iceGatheringState === "complete") return Promise.resolve();
    return new Promise(function (resolve) {
      var settled = false;
      function finish() {
        if (settled) return;
        settled = true;
        peer.removeEventListener("icegatheringstatechange", changed);
        resolve();
      }
      function changed() { if (peer.iceGatheringState === "complete") finish(); }
      peer.addEventListener("icegatheringstatechange", changed);
      window.setTimeout(finish, 8000);
    });
  }

  function waitForChannel(channel) {
    if (channel.readyState === "open") return Promise.resolve();
    return new Promise(function (resolve, reject) {
      var timer = window.setTimeout(function () { reject(new Error("O canal de áudio WebRTC não abriu.")); }, 12000);
      channel.addEventListener("open", function () { window.clearTimeout(timer); resolve(); }, { once: true });
      channel.addEventListener("error", function () { window.clearTimeout(timer); reject(new Error("Falha no canal de áudio WebRTC.")); }, { once: true });
    });
  }

  async function negotiate(instance, callId, preparedAudio) {
    cleanupMedia();
    var audio = preparedAudio || await setupAudio();
    var peer;
    var channel;
    var media;
    try {
      peer = new RTCPeerConnection({});
      channel = peer.createDataChannel("pcm");
      channel.binaryType = "arraybuffer";
      media = Object.assign(audio, { callId: callId, peer: peer, channel: channel });
      state.media = media;
      state.muted = false;

      audio.capture.port.onmessage = function (event) {
        if (state.media !== media || state.muted || channel.readyState !== "open") return;
        if (channel.bufferedAmount > 512 * 1024) return;
        try { channel.send(event.data); } catch (_) {}
      };
      channel.onmessage = function (event) {
        if (state.media !== media) return;
        if (event.data instanceof ArrayBuffer) {
          audio.playback.port.postMessage(event.data, [event.data]);
        } else if (event.data && typeof event.data.arrayBuffer === "function") {
          event.data.arrayBuffer().then(function (buffer) {
            if (state.media === media) audio.playback.port.postMessage(buffer, [buffer]);
          });
        }
      };
      peer.addEventListener("connectionstatechange", function () {
        if (state.media !== media) return;
        if (peer.connectionState === "failed") renderModal("A conexão WebRTC falhou.", true);
      });

      var offer = await peer.createOffer();
      await peer.setLocalDescription(offer);
      await waitForICE(peer);
      var local = peer.localDescription;
      if (!local || !local.sdp) throw new Error("O navegador não gerou uma oferta WebRTC válida.");
      var response = await request(instance, "/call/" + encodeURIComponent(callId) + "/webrtc", {
        method: "POST",
        body: { sdpOffer: local.sdp },
      });
      await peer.setRemoteDescription({ type: "answer", sdp: response.sdpAnswer });
      await waitForChannel(channel);
    } catch (error) {
      if (state.media === media) cleanupMedia();
      else disposeAudio(audio);
      throw error;
    }
  }

  function disposeAudio(audio) {
    if (!audio) return;
    try { audio.capture.port.postMessage({ enabled: false }); } catch (_) {}
    try { audio.stream.getTracks().forEach(function (track) { track.stop(); }); } catch (_) {}
    [audio.source, audio.capture, audio.playback, audio.silent].forEach(function (node) {
      try { if (node) node.disconnect(); } catch (_) {}
    });
    try { audio.context.close(); } catch (_) {}
  }

  function cleanupMedia() {
    var media = state.media;
    state.media = null;
    state.muted = false;
    if (!media) return;
    disposeAudio(media);
    try { media.channel.close(); } catch (_) {}
    try { media.peer.close(); } catch (_) {}
  }

  async function startCall() {
    var instance = getInstance(state.currentInstanceId);
    var input = root().querySelector("[data-wc-number]");
    var number = core.normalizeNumber(input && input.value);
    if (!instance || number.length < 8) {
      renderModal("Informe um número válido com DDI e DDD.", true);
      return;
    }
    setBusy(true, "Solicitando microfone e iniciando chamada...");
    var preparedAudio = null;
    var call = null;
    try {
      preparedAudio = await setupAudio();
      var started = await request(instance, "/call/start", { method: "POST", body: { number: number } });
      call = mergeCall(instance.id, Object.assign({}, started, { instanceId: instance.id, peer: number }));
      var audio = preparedAudio;
      preparedAudio = null;
      await negotiate(instance, call.callId, audio);
      state.message = "Áudio WebRTC conectado.";
      state.messageIsError = false;
    } catch (error) {
      disposeAudio(preparedAudio);
      cleanupMedia();
      if (call && call.callId) {
        try {
          var ended = await request(instance, "/call/" + encodeURIComponent(call.callId), { method: "DELETE" });
          mergeCall(instance.id, ended);
        } catch (_) {}
      }
      state.message = error.name === "NotAllowedError" ? "Permissão do microfone negada." : error.message;
      state.messageIsError = true;
    } finally {
      state.busy = false;
      renderModal();
    }
  }

  async function acceptCall() {
    var instance = getInstance(state.currentInstanceId);
    var call = state.calls.get(state.currentInstanceId);
    if (!instance || !call) return;
    setBusy(true, "Preparando áudio e assumindo chamada...");
    try {
      await negotiate(instance, call.callId);
      var body = await request(instance, "/call/" + encodeURIComponent(call.callId) + "/accept", { method: "POST" });
      mergeCall(instance.id, Object.assign({}, call, body));
      state.message = "Chamada aceita. Áudio conectado.";
      state.messageIsError = false;
    } catch (error) {
      cleanupMedia();
      state.message = error.name === "NotAllowedError" ? "Permissão do microfone negada." : error.message;
      state.messageIsError = true;
    } finally {
      state.busy = false;
      renderModal();
    }
  }

  async function rejectCall(closeAfter) {
    var instance = getInstance(state.currentInstanceId);
    var call = state.calls.get(state.currentInstanceId);
    if (!instance || !call) return;
    setBusy(true, "Rejeitando chamada...");
    try {
      var body = await request(instance, "/call/" + encodeURIComponent(call.callId) + "/reject", { method: "POST" });
      mergeCall(instance.id, body);
      cleanupMedia();
      if (closeAfter) forceCloseModal();
    } catch (error) {
      state.message = error.message;
      state.messageIsError = true;
    } finally {
      state.busy = false;
      if (state.currentInstanceId) renderModal();
    }
  }

  async function endCall(closeAfter) {
    var instance = getInstance(state.currentInstanceId);
    var call = state.calls.get(state.currentInstanceId);
    if (!instance || !call) return;
    setBusy(true, "Encerrando chamada...");
    try {
      var body = await request(instance, "/call/" + encodeURIComponent(call.callId), { method: "DELETE", keepalive: true });
      mergeCall(instance.id, body);
      cleanupMedia();
      if (closeAfter) forceCloseModal();
    } catch (error) {
      state.message = error.message;
      state.messageIsError = true;
    } finally {
      state.busy = false;
      if (state.currentInstanceId) renderModal();
    }
  }

  function toggleMute() {
    state.muted = !state.muted;
    if (state.media && state.media.capture) {
      state.media.capture.port.postMessage({ enabled: !state.muted });
      state.media.stream.getAudioTracks().forEach(function (track) { track.enabled = !state.muted; });
    }
    state.message = state.muted ? "Microfone silenciado." : "Microfone ativado.";
    state.messageIsError = false;
    renderModal();
  }

  var injectScheduled = false;
  function scheduleInject() {
    if (injectScheduled) return;
    injectScheduled = true;
    window.setTimeout(function () {
      injectScheduled = false;
      injectButtons();
    }, 200);
  }

  var observer = new MutationObserver(scheduleInject);
  observer.observe(document.body, { childList: true, subtree: true });
  window.addEventListener("storage", loadInstances);
  window.addEventListener("popstate", function () { loadInstances(); scheduleInject(); });
  window.addEventListener("beforeunload", function () {
    state.streams.forEach(function (entry) { entry.controller.abort(); });
    cleanupMedia();
  });
  window.setInterval(loadInstances, 15000);
  window.setInterval(scheduleInject, 2000);
  window.setTimeout(loadInstances, 500);
  window.setTimeout(loadInstances, 1800);
})();
