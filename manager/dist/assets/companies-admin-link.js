(function () {
  "use strict";

  var targetHref = "/assets/companies-admin.html";
  var linkId = "evolution-companies-admin-link";

  var lastApiKey = null;
  var masterCache = null;
  var masterCheckInFlight = null;
  var observer = null;

  function isManagerPage() {
    return /^\/manager(\/|$)/.test(window.location.pathname);
  }

  function getStoredAuth() {
    try {
      return JSON.parse(localStorage.getItem("evolution-auth") || "{}");
    } catch (err) {
      return {};
    }
  }

  function checkIsMaster(apiKey, apiUrl) {
    if (masterCheckInFlight) return masterCheckInFlight;

    var baseUrl = (apiUrl || window.location.origin).replace(/\/$/, "");

    masterCheckInFlight = fetch(baseUrl + "/company/all", {
      headers: { apikey: apiKey },
    })
      .then(function (response) {
        return response.ok;
      })
      .catch(function () {
        return false;
      })
      .then(function (result) {
        masterCheckInFlight = null;
        return result;
      });

    return masterCheckInFlight;
  }

  function buildIcon() {
    var icon = document.createElement("span");
    icon.setAttribute("aria-hidden", "true");
    icon.textContent = "▦";
    icon.style.display = "inline-flex";
    icon.style.width = "1.25rem";
    icon.style.justifyContent = "center";
    icon.style.fontSize = "1.15rem";
    icon.style.lineHeight = "1";
    return icon;
  }

  function setLabel(link) {
    var spans = Array.prototype.slice.call(link.querySelectorAll("span"));
    var textSpan = spans.find(function (span) {
      return (span.textContent || "").trim() === "Dashboard" || (span.textContent || "").trim() === "Instâncias";
    });

    if (textSpan) {
      textSpan.textContent = "Empresas";
      return;
    }

    link.textContent = "";
    link.appendChild(buildIcon());
    var label = document.createElement("span");
    label.textContent = "Empresas";
    link.appendChild(label);
  }

  function normalizeClass(link) {
    var className = link.getAttribute("class") || "";
    className = className
      .replace(/\bbg-primary\/10\b/g, "")
      .replace(/\btext-primary\b/g, "")
      .replace(/\bbg-green-500\/10\b/g, "")
      .replace(/\btext-green-500\b/g, "")
      .replace(/\s+/g, " ")
      .trim();
    link.setAttribute("class", className);
  }

  function injectLink() {
    if (document.getElementById(linkId)) return true;

    var instancesLink =
      document.querySelector('a[href="/manager/instances"]') ||
      Array.prototype.find.call(document.querySelectorAll("a"), function (link) {
        return (link.textContent || "").indexOf("Instâncias") >= 0;
      });

    if (!instancesLink || !instancesLink.parentNode) return false;

    var link = instancesLink.cloneNode(true);
    link.id = linkId;
    link.href = targetHref;
    link.removeAttribute("aria-current");
    normalizeClass(link);
    setLabel(link);

    instancesLink.parentNode.insertBefore(link, instancesLink.nextSibling);
    return true;
  }

  function removeLink() {
    var link = document.getElementById(linkId);
    if (link && link.parentNode) link.parentNode.removeChild(link);
  }

  function startObserver() {
    if (observer) return;
    observer = new MutationObserver(function () {
      if (document.getElementById(linkId)) return;
      if (masterCache === true) injectLink();
    });
    observer.observe(document.documentElement, { childList: true, subtree: true });
  }

  function stopObserver() {
    if (observer) {
      observer.disconnect();
      observer = null;
    }
  }

  async function evaluate() {
    if (!isManagerPage()) {
      removeLink();
      stopObserver();
      return;
    }

    var auth = getStoredAuth();
    var apiKey = (auth && auth.apiKey) || "";

    if (apiKey !== lastApiKey) {
      lastApiKey = apiKey;
      masterCache = null;
      removeLink();
    }

    if (!apiKey) {
      stopObserver();
      return;
    }

    if (masterCache === null) {
      masterCache = await checkIsMaster(apiKey, auth.apiUrl);
    }

    if (!masterCache) {
      stopObserver();
      return;
    }

    if (!injectLink()) {
      startObserver();
    } else {
      stopObserver();
    }
  }

  function boot() {
    evaluate();
    window.setInterval(evaluate, 2000);

    window.addEventListener("storage", function (event) {
      if (event.key === "evolution-auth") evaluate();
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
