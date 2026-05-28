(function () {
  "use strict";

  var targetHref = "/assets/companies-admin.html";
  var linkId = "evolution-companies-admin-link";
  var isMaster = null;
  var masterCheckPromise = null;

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

  function checkIsMaster() {
    if (isMaster !== null) return Promise.resolve(isMaster);
    if (masterCheckPromise) return masterCheckPromise;

    var auth = getStoredAuth();
    var apiKey = (auth && auth.apiKey) || "";
    if (!apiKey) {
      isMaster = false;
      return Promise.resolve(false);
    }

    var baseUrl = ((auth && auth.apiUrl) || window.location.origin).replace(/\/$/, "");

    masterCheckPromise = fetch(baseUrl + "/company/all", {
      headers: { apikey: apiKey },
    })
      .then(function (response) {
        isMaster = response.ok;
        return isMaster;
      })
      .catch(function () {
        isMaster = false;
        return false;
      });

    return masterCheckPromise;
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
    if (!isManagerPage() || document.getElementById(linkId)) return true;

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

  function startInjection() {
    if (injectLink()) return;

    var observer = new MutationObserver(function () {
      if (injectLink()) observer.disconnect();
    });

    observer.observe(document.documentElement, {
      childList: true,
      subtree: true,
    });

    window.setTimeout(function () {
      observer.disconnect();
    }, 10000);
  }

  function boot() {
    if (!isManagerPage()) return;
    checkIsMaster().then(function (master) {
      if (!master) return;
      startInjection();
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
