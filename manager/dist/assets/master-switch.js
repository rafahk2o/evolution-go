(function () {
  "use strict";

  var STORAGE_KEY = "evolution-selected-company";
  var BANNER_ID = "evolution-master-banner";
  var BANNER_HEIGHT_PX = 44;

  function readSelected() {
    try {
      var raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) return null;
      var parsed = JSON.parse(raw);
      if (!parsed || !parsed.id) return null;
      return parsed;
    } catch (err) {
      return null;
    }
  }

  function clearSelected() {
    try {
      localStorage.removeItem(STORAGE_KEY);
    } catch (err) {
      /* ignore */
    }
  }

  var origFetch = window.fetch ? window.fetch.bind(window) : null;
  if (origFetch) {
    window.fetch = function (resource, init) {
      try {
        var selected = readSelected();
        if (selected && selected.id) {
          var url = "";
          if (typeof resource === "string") {
            url = resource;
          } else if (resource && typeof resource.url === "string") {
            url = resource.url;
          }
          var sameOrigin =
            url.startsWith("/") ||
            url.startsWith(window.location.origin) ||
            url === "";
          if (sameOrigin) {
            init = init || {};
            var headers = new Headers(init.headers || (resource && resource.headers) || {});
            if (!headers.has("X-Company-Id")) {
              headers.set("X-Company-Id", selected.id);
            }
            init.headers = headers;
          }
        }
      } catch (err) {
        /* fall through to fetch */
      }
      return origFetch(resource, init);
    };
  }

  function ensureBanner(selected) {
    var banner = document.getElementById(BANNER_ID);
    if (!selected) {
      if (banner) banner.remove();
      document.body.style.paddingTop = "";
      return;
    }
    if (!banner) {
      banner = document.createElement("div");
      banner.id = BANNER_ID;
      banner.style.cssText = [
        "position:fixed",
        "top:0",
        "left:0",
        "right:0",
        "z-index:9999",
        "background:#001b12",
        "color:#00e995",
        "padding:10px 18px",
        "font-family:Inter,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif",
        "font-size:13px",
        "display:flex",
        "justify-content:space-between",
        "align-items:center",
        "gap:12px",
        "border-bottom:1px solid rgba(0,233,149,0.5)",
        "box-shadow:0 2px 12px rgba(0,233,149,0.18)",
      ].join(";");

      var text = document.createElement("span");
      text.id = BANNER_ID + "-text";
      banner.appendChild(text);

      var exit = document.createElement("button");
      exit.type = "button";
      exit.textContent = "Sair do modo empresa";
      exit.style.cssText = [
        "background:transparent",
        "border:1px solid #00e995",
        "color:#00e995",
        "border-radius:6px",
        "padding:6px 12px",
        "cursor:pointer",
        "font:inherit",
        "font-weight:600",
      ].join(";");
      exit.addEventListener("click", function () {
        clearSelected();
        window.location.href = "/assets/companies-admin.html";
      });
      banner.appendChild(exit);

      document.body.appendChild(banner);
      document.body.style.paddingTop = BANNER_HEIGHT_PX + "px";
    }
    var label = document.getElementById(BANNER_ID + "-text");
    if (label) {
      var name = selected.name ? String(selected.name) : selected.id;
      label.textContent = "Modo master — visualizando empresa: " + name;
    }
  }

  function boot() {
    ensureBanner(readSelected());
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }

  window.addEventListener("storage", function (event) {
    if (event.key === STORAGE_KEY) {
      ensureBanner(readSelected());
    }
  });
})();
