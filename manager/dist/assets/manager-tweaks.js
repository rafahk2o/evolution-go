(function () {
  "use strict";

  var hasSeenContent = false;
  var lastPath = "";
  var pendingFrame = null;

  function isInstancesPage() {
    return /^\/manager\/instances/.test(window.location.pathname);
  }

  function evaluate() {
    pendingFrame = null;

    if (window.location.pathname !== lastPath) {
      lastPath = window.location.pathname;
      hasSeenContent = false;
    }

    if (!isInstancesPage()) return;

    var main = document.querySelector("main");
    if (!main) return;

    var pulses = main.querySelectorAll(".animate-pulse");

    if (pulses.length === 0) {
      hasSeenContent = true;
      return;
    }

    if (hasSeenContent) {
      for (var i = 0; i < pulses.length; i++) {
        pulses[i].style.setProperty("display", "none", "important");
      }
    }
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
