(function () {
  "use strict";

  var lastPath = "";
  var cachedHtml = "";
  var overlay = null;
  var overlayId = "evolution-freeze-overlay";
  var hostContainer = null;
  var hostOriginalPosition = "";
  var pendingFrame = null;

  function isInstancesPage() {
    return /^\/manager\/instances/.test(window.location.pathname);
  }

  function findContentContainer() {
    var main = document.querySelector("main");
    if (!main) return null;
    var area =
      main.querySelector(".flex-1.overflow-auto") ||
      main.querySelector('[class*="overflow-auto"]') ||
      main;
    return area;
  }

  function removeOverlay() {
    if (overlay && overlay.parentNode) overlay.parentNode.removeChild(overlay);
    overlay = null;
    if (hostContainer) {
      if (hostOriginalPosition) {
        hostContainer.style.position = hostOriginalPosition;
      } else {
        hostContainer.style.removeProperty("position");
      }
      hostContainer = null;
      hostOriginalPosition = "";
    }
  }

  function ensureOverlay(container, html) {
    if (!overlay) {
      overlay = document.createElement("div");
      overlay.id = overlayId;
      overlay.setAttribute("aria-hidden", "true");
      overlay.style.position = "absolute";
      overlay.style.top = "0";
      overlay.style.left = "0";
      overlay.style.right = "0";
      overlay.style.bottom = "0";
      overlay.style.zIndex = "5";
      overlay.style.background = "var(--background, #0a0a0a)";
      overlay.style.pointerEvents = "none";
      overlay.style.overflow = "auto";

      hostContainer = container;
      hostOriginalPosition = container.style.position;
      var computed = window.getComputedStyle(container).position;
      if (computed === "static") container.style.position = "relative";

      container.appendChild(overlay);
    }
    if (overlay.innerHTML !== html) overlay.innerHTML = html;
  }

  function evaluate() {
    pendingFrame = null;

    if (window.location.pathname !== lastPath) {
      lastPath = window.location.pathname;
      cachedHtml = "";
      removeOverlay();
    }

    if (!isInstancesPage()) {
      removeOverlay();
      return;
    }

    var container = findContentContainer();
    if (!container) return;

    var pulse = container.querySelector(".animate-pulse");

    if (!pulse) {
      cachedHtml = container.innerHTML;
      removeOverlay();
      return;
    }

    if (!cachedHtml) return;

    ensureOverlay(container, cachedHtml);
  }

  function scheduleEvaluate() {
    if (pendingFrame !== null) return;
    pendingFrame = window.requestAnimationFrame(evaluate);
  }

  function boot() {
    evaluate();
    var observer = new MutationObserver(scheduleEvaluate);
    observer.observe(document.documentElement, {
      childList: true,
      subtree: true,
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
