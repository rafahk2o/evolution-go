(function (root, factory) {
  "use strict";

  var api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  } else {
    root.WaCallsWidgetCore = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  var terminalStatuses = {
    ended: true,
    rejected: true,
    failed: true,
  };

  var defaultErrors = {
    400: "Dados da chamada ou identificador do navegador inválidos.",
    404: "A chamada não existe mais ou expirou.",
    409: "Outro operador assumiu a chamada ou existe uma chamada em andamento.",
    422: "Número ou negociação WebRTC inválidos.",
    503: "A instância está desconectada ou a mídia está indisponível.",
    504: "A negociação WebRTC excedeu o tempo limite.",
  };

  function normalizeNumber(value) {
    return String(value == null ? "" : value).replace(/\D/g, "");
  }

  function isTerminalStatus(status) {
    return !!terminalStatuses[String(status || "").toLowerCase()];
  }

  function callErrorMessage(status, body) {
    body = body || {};
    if (typeof body.error === "string" && body.error.trim()) return body.error.trim();
    if (typeof body.message === "string" && body.message.trim()) return body.message.trim();
    return defaultErrors[Number(status)] || "Falha inesperada ao controlar a chamada.";
  }

  function normalizePeer(peer) {
    return String(peer || "").split("@")[0];
  }

  function normalizeCall(value) {
    value = value && value.call ? value.call : value || {};
    return {
      callId: String(value.callId || value.id || ""),
      instanceId: String(value.instanceId || ""),
      clientId: String(value.clientId || ""),
      direction: String(value.direction || ""),
      status: String(value.status || ""),
      peer: normalizePeer(value.peer || value.number || value.callCreator),
      timestamp: String(value.timestamp || ""),
    };
  }

  function createSSEParser(onEvent) {
    var buffer = "";

    function emit(block) {
      var eventName = "message";
      var dataLines = [];
      block.split(/\r?\n/).forEach(function (line) {
        if (!line || line.charAt(0) === ":") return;
        var separator = line.indexOf(":");
        var field = separator === -1 ? line : line.slice(0, separator);
        var value = separator === -1 ? "" : line.slice(separator + 1).replace(/^ /, "");
        if (field === "event") eventName = value || "message";
        if (field === "data") dataLines.push(value);
      });
      if (!dataLines.length) return;
      var raw = dataLines.join("\n");
      var data = raw;
      try {
        data = JSON.parse(raw);
      } catch (_) {
        // Non-JSON SSE data is preserved for diagnostics.
      }
      onEvent({ event: eventName, data: data });
    }

    return {
      push: function (chunk) {
        buffer += String(chunk == null ? "" : chunk);
        var match;
        while ((match = /\r?\n\r?\n/.exec(buffer))) {
          var block = buffer.slice(0, match.index);
          buffer = buffer.slice(match.index + match[0].length);
          emit(block);
        }
      },
      end: function () {
        if (buffer.trim()) emit(buffer);
        buffer = "";
      },
    };
  }

  return {
    normalizeNumber: normalizeNumber,
    isTerminalStatus: isTerminalStatus,
    callErrorMessage: callErrorMessage,
    normalizeCall: normalizeCall,
    createSSEParser: createSSEParser,
  };
});
