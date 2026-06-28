(function (root, factory) {
  "use strict";

  var api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.WaCallsCore = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  var terminalStatuses = { ended: true, rejected: true, failed: true };
  var errorMessages = {
    400: "Dados da chamada ou identificador do navegador inválidos.",
    401: "API key inválida ou expirada.",
    403: "A API key não permite esta operação.",
    404: "A chamada não existe mais ou expirou.",
    409: "Já existe uma chamada em andamento para este navegador.",
    422: "Número ou negociação WebRTC inválidos.",
    503: "A instância está desconectada ou a mídia está indisponível.",
    504: "A negociação WebRTC excedeu o tempo limite.",
  };

  function normalizeApiOrigin(value) {
    var url;
    try {
      url = new URL(String(value == null ? "" : value).trim());
    } catch (_) {
      throw new Error("Informe uma URL válida da Evolution GO.");
    }
    if (url.username || url.password) {
      throw new Error("A URL da API não pode conter credenciais.");
    }
    if (url.pathname !== "/" || url.search || url.hash) {
      throw new Error("Informe somente a origem da API, sem caminho, consulta ou fragmento.");
    }
    var loopback = url.hostname === "localhost" || url.hostname === "127.0.0.1";
    if (url.protocol !== "https:" && !(url.protocol === "http:" && loopback)) {
      throw new Error("A URL da API deve usar HTTPS; HTTP é permitido apenas em localhost.");
    }
    return url.origin;
  }

  function permissionPattern(value) {
    var url = new URL(normalizeApiOrigin(value));
    return url.protocol + "//" + url.hostname + "/*";
  }

  function normalizePhone(value) {
    var number = String(value == null ? "" : value).replace(/\D/g, "");
    if (number.length < 8 || number.length > 15) {
      throw new Error("Informe um número válido com DDI e DDD.");
    }
    return number;
  }

  function normalizeInstanceStatus(value) {
    var data = value && value.data ? value.data : value || {};
    return {
      connected: data.Connected === true || data.connected === true,
      loggedIn: data.LoggedIn === true || data.loggedIn === true,
      instanceName: String(data.Name || data.name || "").trim(),
    };
  }

  function normalizeCall(value) {
    var call = value && value.call ? value.call : value || {};
    return {
      callId: String(call.callId || call.id || ""),
      direction: String(call.direction || ""),
      status: String(call.status || "").toLowerCase(),
      peer: String(call.peer || call.number || call.callCreator || "").split("@")[0],
    };
  }

  function isTerminalStatus(status) {
    return !!terminalStatuses[String(status || "").toLowerCase()];
  }

  function boundedMessage(value) {
    if (typeof value !== "string") return "";
    var message = value.trim();
    return message && message.length <= 240 ? message : "";
  }

  function callErrorMessage(status, body, error) {
    body = body || {};
    var upstream = boundedMessage(body.error) || boundedMessage(body.message);
    if (upstream) return upstream;
    if (error && (error.name === "TypeError" || /fetch|network/i.test(String(error.message || "")))) {
      return "Não foi possível acessar a Evolution GO.";
    }
    return errorMessages[Number(status)] || "Falha inesperada ao controlar a chamada.";
  }

  return {
    normalizeApiOrigin: normalizeApiOrigin,
    permissionPattern: permissionPattern,
    normalizePhone: normalizePhone,
    normalizeInstanceStatus: normalizeInstanceStatus,
    normalizeCall: normalizeCall,
    isTerminalStatus: isTerminalStatus,
    callErrorMessage: callErrorMessage,
  };
});
