(function () {
  "use strict";

  var state = {
    instances: [],
    loading: false,
    current: null,
    settings: null,
    advanced: null,
    webhookUrl: "",
  };

  var css = ""
    + ".cw-chatwoot-card{position:relative;}"
    + ".cw-card-button{position:absolute;top:0;right:0;z-index:5;display:inline-flex;align-items:center;justify-content:center;gap:4px;height:18px;padding:0 6px;border:1px solid rgba(37,211,102,.35);border-top:0;border-right:0;border-radius:0 0 0 6px;background:rgba(17,24,39,.96);color:#25d366;font-weight:700;font-size:10px;line-height:1;white-space:nowrap;cursor:pointer;opacity:0;pointer-events:none;transform:translateY(-4px);transition:opacity .15s ease,transform .15s ease,background .15s ease,border-color .15s ease;}"
    + ".cw-card-button img{width:12px;height:12px;object-fit:contain;}"
    + ".cw-chatwoot-card:hover .cw-card-button,.cw-chatwoot-card:focus-within .cw-card-button{opacity:1;pointer-events:auto;transform:translateY(0);}"
    + ".cw-card-button:hover{background:rgba(37,211,102,.12);border-color:rgba(37,211,102,.65);}"
    + ".cw-overlay{position:fixed;inset:0;z-index:9999;display:flex;align-items:center;justify-content:center;background:rgba(0,0,0,.72);padding:16px;}"
    + ".cw-modal{width:min(620px,100%);max-height:92vh;overflow:auto;background:#111827;border:1px solid #334155;border-radius:8px;color:#e5e7eb;box-shadow:0 24px 80px rgba(0,0,0,.55);}"
    + ".cw-header{display:flex;align-items:center;justify-content:space-between;padding:14px 18px;border-bottom:1px solid #263241;position:sticky;top:0;background:#111827;z-index:1;}"
    + ".cw-title{font-size:16px;font-weight:800;}"
    + ".cw-close{border:0;background:transparent;color:#9ca3af;font-size:24px;line-height:1;cursor:pointer;}"
    + ".cw-body{padding:16px 18px 18px;}"
    + ".cw-section{border-top:1px solid #263241;padding-top:14px;margin-top:14px;}"
    + ".cw-section-title{text-align:center;font-weight:800;font-size:14px;color:#f9fafb;margin-bottom:10px;}"
    + ".cw-grid{display:grid;grid-template-columns:1fr 1fr;gap:10px;}"
    + ".cw-field{display:flex;flex-direction:column;gap:5px;margin-bottom:10px;}"
    + ".cw-field label{font-size:12px;font-weight:700;color:#f3f4f6;}"
    + ".cw-field input,.cw-field select{height:34px;border:1px solid #334155;border-radius:4px;background:#0b1220;color:#e5e7eb;padding:0 10px;font-size:13px;outline:none;}"
    + ".cw-field input:focus,.cw-field select:focus{border-color:#22c55e;box-shadow:0 0 0 2px rgba(34,197,94,.12);}"
    + ".cw-help{font-size:11px;color:#94a3b8;line-height:1.4;}"
    + ".cw-row{display:flex;align-items:center;gap:8px;}"
    + ".cw-row input{flex:1;}"
    + ".cw-copy,.cw-toggle,.cw-save,.cw-cancel{border:0;border-radius:5px;height:34px;padding:0 12px;font-weight:800;font-size:12px;cursor:pointer;}"
    + ".cw-copy,.cw-save{background:#2dd4a0;color:#062016;}"
    + ".cw-cancel,.cw-toggle{background:#1f2937;color:#e5e7eb;border:1px solid #374151;}"
    + ".cw-switch{display:inline-flex;align-items:center;gap:8px;margin:4px 0 8px;}"
    + ".cw-switch input{width:16px;height:16px;accent-color:#22c55e;}"
    + ".cw-footer{display:flex;justify-content:flex-end;gap:8px;border-top:1px solid #263241;padding:12px 18px;position:sticky;bottom:0;background:#111827;}"
    + ".cw-alert{padding:10px 12px;border-radius:5px;margin-bottom:12px;font-size:12px;}"
    + ".cw-alert.error{background:rgba(239,68,68,.12);border:1px solid rgba(239,68,68,.35);color:#fecaca;}"
    + ".cw-alert.ok{background:rgba(34,197,94,.12);border:1px solid rgba(34,197,94,.35);color:#bbf7d0;}"
    + ".cw-loading{padding:28px;text-align:center;color:#cbd5e1;}"
    + "@media(max-width:640px){.cw-grid{grid-template-columns:1fr}.cw-modal{max-height:96vh}}";

  var style = document.createElement("style");
  style.textContent = css;
  document.head.appendChild(style);

  function authState() {
    try {
      var raw = localStorage.getItem("evolution-auth");
      var parsed = raw ? JSON.parse(raw) : {};
      var data = parsed.state || parsed;
      return {
        apiUrl: (data.apiUrl || window.location.origin).replace(/\/$/, ""),
        apiKey: data.apiKey || "",
      };
    } catch (_) {
      return { apiUrl: window.location.origin, apiKey: "" };
    }
  }

  function normalizeInstance(item) {
    return {
      id: item.id || item.instanceId || item.instanceID || "",
      name: item.name || item.instanceName || item.instance || "",
      token: item.token || item.apikey || item.apiKey || "",
      raw: item,
    };
  }

  function request(path, options, token) {
    var auth = authState();
    var headers = Object.assign({ "Content-Type": "application/json" }, options && options.headers ? options.headers : {});
    if (token || auth.apiKey) headers.apikey = token || auth.apiKey;
    return fetch(auth.apiUrl + path, Object.assign({}, options || {}, { headers: headers })).then(function (res) {
      return res.text().then(function (text) {
        var body = text ? JSON.parse(text) : {};
        if (!res.ok) {
          var msg = body.error || body.message || res.statusText;
          throw new Error(msg);
        }
        return body;
      });
    });
  }

  function loadInstances() {
    if (state.loading) return;
    var auth = authState();
    if (!auth.apiKey) return;
    state.loading = true;
    request("/instance/all", {}, auth.apiKey)
      .then(function (body) {
        var list = Array.isArray(body) ? body : body.data || body.instances || [];
        state.instances = list.map(normalizeInstance).filter(function (instance) {
          return instance.id && instance.name;
        });
        injectButtons();
      })
      .catch(function (err) {
        console.warn("[Chatwoot connector] failed to load instances:", err.message);
      })
      .finally(function () {
        state.loading = false;
      });
  }

  function findCard(instance) {
    var nodes = Array.prototype.slice.call(document.querySelectorAll("main div, body div"));
    var matches = nodes.filter(function (node) {
      if (node.id === "cw-modal-root" || node.closest("#cw-modal-root")) return false;
      var rect = node.getBoundingClientRect();
      if (rect.width < 180 || rect.height < 120) return false;
      if (rect.width > 640 || rect.height > 520) return false;
      var text = node.innerText || "";
      return text.indexOf(instance.name) !== -1 && text.indexOf("Status") !== -1;
    });
    matches.sort(function (a, b) {
      var ar = a.getBoundingClientRect();
      var br = b.getBoundingClientRect();
      return (ar.width * ar.height) - (br.width * br.height);
    });
    return matches[0] || null;
  }

  function actionRow(card) {
    var buttons = Array.prototype.slice.call(card.querySelectorAll("button"));
    if (!buttons.length) return card;
    var rows = buttons.map(function (button) {
      return button.parentElement;
    }).filter(Boolean);
    rows.sort(function (a, b) {
      return b.querySelectorAll("button").length - a.querySelectorAll("button").length;
    });
    return rows[0] || buttons[buttons.length - 1].parentElement || card;
  }

  function injectButtons() {
    if (!/\/manager/.test(window.location.pathname)) return;
    state.instances.forEach(function (instance) {
      var card = findCard(instance);
      if (!card) return;
      card.classList.add("cw-chatwoot-card");
      var button = document.querySelector('[data-chatwoot-button-for="' + instance.id + '"]');
      if (!button) {
        button = document.createElement("button");
        button.type = "button";
        button.className = "cw-card-button";
        button.dataset.chatwootButtonFor = instance.id;
        button.title = "Configurar Chatwoot";
        button.innerHTML = '<img src="/assets/chatwoot-icon.png" alt=""> <span>Chatwoot</span>';
        button.addEventListener("click", function (event) {
          event.preventDefault();
          event.stopPropagation();
          openModal(instance);
        });
      }
      if (button.parentElement !== card) {
        card.appendChild(button);
      }
    });
  }

  function defaultSettings(instance) {
    return {
      enabled: false,
      url: "https://app.melck.app",
      accountId: "1",
      accountToken: "",
      inboxId: "",
      inboxIdentifier: "",
      webhookToken: instance.name,
      hmacToken: "",
      enableGroups: false,
    };
  }

  function defaultAdvanced() {
    return {
      alwaysOnline: false,
      rejectCall: false,
      msgRejectCall: "",
      readMessages: false,
      ignoreGroups: false,
      ignoreStatus: false,
    };
  }

  function computedWebhook(instance, settings) {
    var token = settings.webhookToken || instance.token || instance.name;
    return window.location.origin.replace(/\/$/, "") + "/webhooks/chatwoot/" + encodeURIComponent(instance.name) + "/" + encodeURIComponent(token);
  }

  function root() {
    var el = document.getElementById("cw-modal-root");
    if (!el) {
      el = document.createElement("div");
      el.id = "cw-modal-root";
      document.body.appendChild(el);
    }
    return el;
  }

  function esc(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function openModal(instance) {
    state.current = instance;
    state.settings = defaultSettings(instance);
    state.advanced = defaultAdvanced();
    state.webhookUrl = computedWebhook(instance, state.settings);
    renderLoading(instance);

    Promise.all([
      request("/instance/" + encodeURIComponent(instance.id) + "/chatwoot-settings", {}, instance.token),
      request("/instance/" + encodeURIComponent(instance.id) + "/advanced-settings", {}, instance.token),
    ])
      .then(function (results) {
        state.settings = Object.assign(defaultSettings(instance), results[0].settings || results[0] || {});
        state.advanced = Object.assign(defaultAdvanced(), results[1] || {});
        state.webhookUrl = results[0].webhookUrl || computedWebhook(instance, state.settings);
        renderModal();
      })
      .catch(function (err) {
        renderModal(err.message);
      });
  }

  function closeModal() {
    root().innerHTML = "";
  }

  function renderLoading(instance) {
    root().innerHTML = '<div class="cw-overlay"><div class="cw-modal"><div class="cw-header"><div class="cw-title">Chatwoot - ' + esc(instance.name) + '</div><button class="cw-close" type="button" data-cw-close>&times;</button></div><div class="cw-loading">Carregando configuracao...</div></div></div>';
    root().querySelector("[data-cw-close]").addEventListener("click", closeModal);
  }

  function renderModal(message, ok) {
    var instance = state.current;
    var s = state.settings || defaultSettings(instance);
    var a = state.advanced || defaultAdvanced();
    var webhook = computedWebhook(instance, s);
    state.webhookUrl = webhook;

    root().innerHTML = ''
      + '<div class="cw-overlay">'
      + '<div class="cw-modal">'
      + '<div class="cw-header"><div class="cw-title">Edit App</div><button class="cw-close" type="button" data-cw-close>&times;</button></div>'
      + '<div class="cw-body">'
      + (message ? '<div class="cw-alert ' + (ok ? 'ok' : 'error') + '">' + esc(message) + '</div>' : '')
      + '<div class="cw-field"><label>App ID</label><input data-cw="appId" value="' + esc(instance.name) + '" disabled></div>'
      + '<div class="cw-field"><label>App Type</label><select disabled><option>ChatWoot</option></select></div>'
      + '<label class="cw-switch"><input data-cw="enabled" type="checkbox" ' + (s.enabled ? "checked" : "") + '> <span>Enabled</span></label>'
      + '<div class="cw-section"><div class="cw-section-title">ChatWoot Inbox - Webhook URL</div>'
      + '<div class="cw-field"><label>Use this Webhook URL for your ChatWoot Inbox:</label><div class="cw-row"><input data-cw="webhook" value="' + esc(webhook) + '" readonly><button class="cw-copy" type="button" data-cw-copy="webhook">Copy</button></div><div class="cw-help">Configure esta URL no Webhook da API Inbox do Chatwoot.</div></div>'
      + '</div>'
      + '<div class="cw-section"><div class="cw-section-title">Connection</div>'
      + '<div class="cw-field"><label>ChatWoot URL</label><input data-cw="url" value="' + esc(s.url) + '" placeholder="https://app.melck.app"></div>'
      + '<div class="cw-grid">'
      + '<div class="cw-field"><label>Account ID</label><input data-cw="accountId" value="' + esc(s.accountId) + '"></div>'
      + '<div class="cw-field"><label>Account Token</label><input data-cw="accountToken" type="password" value="' + esc(s.accountToken) + '"></div>'
      + '<div class="cw-field"><label>Inbox ID</label><input data-cw="inboxId" value="' + esc(s.inboxId) + '"></div>'
      + '<div class="cw-field"><label>Inbox Identifier</label><input data-cw="inboxIdentifier" type="password" value="' + esc(s.inboxIdentifier) + '"></div>'
      + '</div>'
      + '<div class="cw-grid">'
      + '<div class="cw-field"><label>Webhook Token</label><input data-cw="webhookToken" value="' + esc(s.webhookToken) + '"></div>'
      + '<div class="cw-field"><label>HMAC Token</label><input data-cw="hmacToken" type="password" value="' + esc(s.hmacToken) + '"></div>'
      + '</div>'
      + '</div>'
      + '<div class="cw-section"><div class="cw-section-title">Conversations</div>'
      + '<label class="cw-switch"><input data-cw="readMessages" type="checkbox" ' + (a.readMessages ? "checked" : "") + '> <span>Confirmacao de leitura no WhatsApp (riscos azuis)</span></label>'
      + '<div class="cw-help">Quando ativado, mensagens recebidas no WhatsApp sao marcadas como lidas automaticamente pela instancia.</div>'
      + '<label class="cw-switch"><input data-cw="enableGroups" type="checkbox" ' + (s.enableGroups ? "checked" : "") + '> <span>Sincronizar grupos</span></label>'
      + '</div>'
      + '</div>'
      + '<div class="cw-footer"><button class="cw-cancel" type="button" data-cw-close>Cancel</button><button class="cw-save" type="button" data-cw-save>Save</button></div>'
      + '</div></div>';

    Array.prototype.slice.call(root().querySelectorAll("[data-cw-close]")).forEach(function (button) {
      button.addEventListener("click", closeModal);
    });
    root().querySelector("[data-cw-save]").addEventListener("click", saveModal);
    root().querySelector("[data-cw-copy]").addEventListener("click", function () {
      var input = root().querySelector('[data-cw="webhook"]');
      navigator.clipboard.writeText(input.value);
      renderModal("Webhook copiado.", true);
    });
    root().querySelector('[data-cw="webhookToken"]').addEventListener("input", function () {
      var next = collectSettings();
      root().querySelector('[data-cw="webhook"]').value = computedWebhook(instance, next);
    });
  }

  function value(name) {
    var el = root().querySelector('[data-cw="' + name + '"]');
    if (!el) return "";
    return el.type === "checkbox" ? el.checked : el.value.trim();
  }

  function collectSettings() {
    return {
      enabled: !!value("enabled"),
      url: value("url"),
      accountId: value("accountId"),
      accountToken: value("accountToken"),
      inboxId: value("inboxId"),
      inboxIdentifier: value("inboxIdentifier"),
      webhookToken: value("webhookToken"),
      hmacToken: value("hmacToken"),
      enableGroups: !!value("enableGroups"),
    };
  }

  function saveModal() {
    var instance = state.current;
    var settings = collectSettings();
    var advanced = Object.assign(defaultAdvanced(), state.advanced || {}, {
      readMessages: !!value("readMessages"),
    });

    if (settings.enabled && (!settings.url || !settings.inboxIdentifier)) {
      renderModal("Informe ChatWoot URL e Inbox Identifier para ativar.", false);
      return;
    }

    root().querySelector("[data-cw-save]").disabled = true;
    Promise.all([
      request("/instance/" + encodeURIComponent(instance.id) + "/chatwoot-settings", {
        method: "PUT",
        body: JSON.stringify(settings),
      }, instance.token),
      request("/instance/" + encodeURIComponent(instance.id) + "/advanced-settings", {
        method: "PUT",
        body: JSON.stringify(advanced),
      }, instance.token),
    ])
      .then(function () {
        state.settings = settings;
        state.advanced = advanced;
        closeModal();
        loadInstances();
      })
      .catch(function (err) {
        renderModal(err.message, false);
      });
  }

  function boot() {
    loadInstances();
    injectButtons();
  }

  var observer = new MutationObserver(function () {
    injectButtons();
  });
  observer.observe(document.body, { childList: true, subtree: true });

  window.addEventListener("storage", loadInstances);
  window.addEventListener("popstate", boot);
  setInterval(loadInstances, 15000);
  setTimeout(boot, 700);
  setTimeout(boot, 2000);
})();
