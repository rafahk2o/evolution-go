(function (root, factory) {
  "use strict";

  var api;
  if (typeof module === "object" && module.exports) {
    api = factory(require("./core.js"));
    module.exports = api;
  } else {
    api = factory(root.WaCallsCore);
    root.WaCallsProtocol = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function (core) {
  "use strict";

  var TYPES = Object.freeze({
    CONFIG_GET: "CONFIG_GET",
    CONFIG_SAVE: "CONFIG_SAVE",
    CALL_START: "CALL_START",
    CALL_WEBRTC: "CALL_WEBRTC",
    CALL_ACTIVE: "CALL_ACTIVE",
    CALL_END: "CALL_END",
  });
  var callIdPattern = /^[A-Za-z0-9._:@+-]{1,256}$/;

  function exactKeys(message, allowed) {
    Object.keys(message).forEach(function (key) {
      if (allowed.indexOf(key) === -1) throw new Error("Campo não permitido: " + key + ".");
    });
  }

  function validCallId(value) {
    var callId = String(value == null ? "" : value).trim();
    if (!callIdPattern.test(callId)) throw new Error("Informe um call ID válido.");
    return callId;
  }

  function validateMessage(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      throw new Error("Mensagem interna inválida.");
    }
    var type = String(value.type || "");
    var result;
    switch (type) {
      case TYPES.CONFIG_GET:
      case TYPES.CALL_ACTIVE:
        exactKeys(value, ["type"]);
        result = { type: type };
        break;
      case TYPES.CONFIG_SAVE: {
        exactKeys(value, ["type", "apiUrl", "apiKey"]);
        var apiKey = String(value.apiKey == null ? "" : value.apiKey).trim();
        if (!apiKey || apiKey.length > 4096) throw new Error("Informe uma API key válida.");
        result = { type: type, apiUrl: core.normalizeApiOrigin(value.apiUrl), apiKey: apiKey };
        break;
      }
      case TYPES.CALL_START:
        exactKeys(value, ["type", "number"]);
        result = { type: type, number: core.normalizePhone(value.number) };
        break;
      case TYPES.CALL_WEBRTC: {
        exactKeys(value, ["type", "callId", "sdpOffer"]);
        var sdpOffer = String(value.sdpOffer == null ? "" : value.sdpOffer);
        if (!sdpOffer || sdpOffer.length > 262144) throw new Error("A oferta SDP é inválida ou excede o limite.");
        result = { type: type, callId: validCallId(value.callId), sdpOffer: sdpOffer };
        break;
      }
      case TYPES.CALL_END:
        exactKeys(value, ["type", "callId"]);
        result = { type: type, callId: validCallId(value.callId) };
        break;
      default:
        throw new Error("Operação interna não suportada.");
    }
    return Object.freeze(result);
  }

  return { TYPES: TYPES, validateMessage: validateMessage };
});
