/**
 * Suporte à cerimônia de passkey (WebAuthn) no manager.
 *
 * O bundle deste manager é anterior ao 0.7.2 e só sabe renderizar QR code. Quando
 * o WhatsApp exige passkey para vincular a conta, GET /instance/qr não devolve
 * `qrcode`, e sim `passkeyStage` + `passkeyOpenUrl` — e o modal fica parado em
 * "Aguardando QR Code...".
 *
 * Este script observa as respostas de /instance/qr (sem alterá-las) e, quando
 * detecta uma cerimônia ativa, injeta no modal o botão "Abrir WhatsApp Web" que
 * dispara a cerimônia. Solução provisória: o correto é rebuildar o manager a
 * partir do fonte do 0.7.2, que já traz essa UI nativamente.
 */
(function () {
  "use strict";

  var PANEL_ID = "evo-passkey-panel";
  var QR_PATH = "/instance/qr";

  var current = null; // { stage, openUrl, code, at }

  function isQrUrl(url) {
    return typeof url === "string" && url.indexOf(QR_PATH) !== -1;
  }

  function readPayload(text) {
    try {
      var body = JSON.parse(text);
      var data = body && body.data ? body.data : body;
      if (data && data.passkeyOpenUrl) {
        return {
          stage: data.passkeyStage || "",
          openUrl: data.passkeyOpenUrl,
          code: data.passkeyCode || "",
          at: Date.now(),
        };
      }
    } catch (err) {
      /* resposta não-JSON: ignora */
    }
    return null;
  }

  // ---- intercepta fetch (somente leitura: clona a resposta) ----
  var origFetch = window.fetch ? window.fetch.bind(window) : null;
  if (origFetch) {
    window.fetch = function (resource, init) {
      var url = typeof resource === "string" ? resource : resource && resource.url;
      var promise = origFetch(resource, init);
      if (isQrUrl(url)) {
        promise
          .then(function (response) {
            response
              .clone()
              .text()
              .then(function (text) {
                var found = readPayload(text);
                if (found) {
                  current = found;
                  render();
                }
              })
              .catch(function () {});
            return response;
          })
          .catch(function () {});
      }
      return promise;
    };
  }

  // ---- intercepta XMLHttpRequest (o manager usa axios) ----
  if (window.XMLHttpRequest && XMLHttpRequest.prototype) {
    var origOpen = XMLHttpRequest.prototype.open;
    XMLHttpRequest.prototype.open = function (method, url) {
      try {
        this.__evoPasskeyUrl = typeof url === "string" ? url : "";
        if (isQrUrl(this.__evoPasskeyUrl)) {
          this.addEventListener("load", function () {
            var found = readPayload(this.responseText);
            if (found) {
              current = found;
              render();
            }
          });
        }
      } catch (err) {
        /* ignora */
      }
      return origOpen.apply(this, arguments);
    };
  }

  function findModal() {
    var nodes = document.querySelectorAll("div");
    for (var i = 0; i < nodes.length; i++) {
      var text = nodes[i].textContent || "";
      if (text.indexOf("Aguardando QR Code") !== -1 || text.indexOf("Conectar WhatsApp") !== -1) {
        // pega o container mais interno que ainda contém a mensagem
        if (nodes[i].children.length <= 12) return nodes[i];
      }
    }
    return null;
  }

  function render() {
    if (!current) return;

    // a cerimônia expira; não deixa painel velho na tela
    if (Date.now() - current.at > 5 * 60 * 1000) {
      current = null;
      var stale = document.getElementById(PANEL_ID);
      if (stale) stale.remove();
      return;
    }

    var host = findModal();
    if (!host) return;

    var panel = document.getElementById(PANEL_ID);
    if (!panel) {
      panel = document.createElement("div");
      panel.id = PANEL_ID;
      panel.style.cssText = [
        "margin:16px 0",
        "padding:16px",
        "border-radius:8px",
        "border:1px solid #22c55e",
        "background:rgba(34,197,94,0.08)",
        "font-size:14px",
        "line-height:1.5",
        "text-align:left",
      ].join(";");
      host.appendChild(panel);
    }

    panel.innerHTML = "";

    var title = document.createElement("div");
    title.style.cssText = "font-weight:600;margin-bottom:8px";
    title.textContent = "Esta conta exige passkey para vincular";
    panel.appendChild(title);

    var desc = document.createElement("div");
    desc.style.cssText = "margin-bottom:12px;opacity:0.85";
    desc.textContent =
      "O WhatsApp não envia QR Code para esta conta. Abra o link abaixo em um navegador com a extensão Evolution Passkey Helper instalada e confirme com o seu autenticador." +
      (current.stage ? " Etapa atual: " + current.stage + "." : "");
    panel.appendChild(desc);

    if (current.code) {
      var code = document.createElement("div");
      code.style.cssText = "margin-bottom:12px;font-family:monospace";
      code.textContent = "Código: " + current.code;
      panel.appendChild(code);
    }

    var link = document.createElement("a");
    link.href = current.openUrl;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.textContent = "Abrir WhatsApp Web";
    link.style.cssText = [
      "display:inline-block",
      "padding:10px 16px",
      "border-radius:6px",
      "background:#16a34a",
      "color:#fff",
      "font-weight:600",
      "text-decoration:none",
    ].join(";");
    panel.appendChild(link);
  }

  // o modal é remontado pelo React; reaplica o painel quando ele reaparecer
  if (window.MutationObserver) {
    new MutationObserver(function () {
      if (current && !document.getElementById(PANEL_ID)) render();
    }).observe(document.documentElement, { childList: true, subtree: true });
  }
})();
