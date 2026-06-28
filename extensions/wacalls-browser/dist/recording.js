(function (root, factory) {
  "use strict";
  var api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  else root.WaCallsRecording = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function createRecordingManager(deps) {
    var recorder = null;
    var chunks = [];
    var result = null;
    var stopPromise = null;
    var resolveStop = null;
    var state = resetState();

    function resetState() {
      return { status: "inactive", bytes: 0, available: false, filename: "", mimeType: "", limitReached: false, error: "" };
    }
    function snapshot() { return Object.assign({}, state); }
    function emit() { if (deps.onState) deps.onState(snapshot()); }
    function update(values) { Object.assign(state, values); emit(); }

    function mimeType() {
      if (!deps.MediaRecorder || typeof deps.MediaRecorder.isTypeSupported !== "function") return "";
      return ["audio/webm;codecs=opus", "audio/webm"].find(function (value) {
        return deps.MediaRecorder.isTypeSupported(value);
      }) || "";
    }

    function filename(phone) {
      var date = deps.now();
      function pad(value) { return String(value).padStart(2, "0"); }
      var stamp = date.getUTCFullYear() + pad(date.getUTCMonth() + 1) + pad(date.getUTCDate()) + "-" +
        pad(date.getUTCHours()) + pad(date.getUTCMinutes()) + pad(date.getUTCSeconds());
      return "evolution-call-" + String(phone || "").replace(/\D/g, "") + "-" + stamp + ".webm";
    }

    function finalize() {
      var blob = chunks.length ? new deps.Blob(chunks, { type: state.mimeType }) : null;
      if (!blob || !blob.size) {
        recorder = null;
        chunks = [];
        result = null;
        state = resetState();
        emit();
        if (resolveStop) resolveStop(null);
        resolveStop = null;
        return;
      }
      result = { blob: blob, filename: state.filename };
      recorder = null;
      chunks = [];
      update({ status: "ready", bytes: blob.size, available: true });
      if (resolveStop) resolveStop(result);
      resolveStop = null;
    }

    function fail(error) {
      if (recorder) {
        recorder.ondataavailable = null;
        recorder.onstop = function () {};
        recorder.onerror = null;
      }
      recorder = null;
      chunks = [];
      result = null;
      update({ status: "failed", available: false, error: String(error && error.message || "Falha ao gravar a chamada.") });
      if (resolveStop) resolveStop(null);
      resolveStop = null;
    }

    function start(stream, phone) {
      discard();
      var selected = mimeType();
      if (!selected) {
        update({ status: "unavailable", error: "Gravação não suportada neste navegador." });
        return false;
      }
      try {
        recorder = new deps.MediaRecorder(stream, { mimeType: selected, audioBitsPerSecond: 64000 });
        state = Object.assign(resetState(), { status: "recording", filename: filename(phone), mimeType: selected });
        recorder.ondataavailable = function (event) {
          var data = event && event.data;
          if (!data || !data.size) return;
          chunks.push(data);
          state.bytes += data.size;
          emit();
          if (state.bytes >= deps.maxBytes && recorder && recorder.state === "recording") {
            state.limitReached = true;
            stop();
          }
        };
        recorder.onstop = finalize;
        recorder.onerror = function (event) { fail(event && event.error || new Error("Falha ao gravar a chamada.")); };
        recorder.start(1000);
        emit();
        return true;
      } catch (error) {
        fail(error);
        return false;
      }
    }

    function stop() {
      if (state.status === "ready") return Promise.resolve(result);
      if (stopPromise) return stopPromise;
      if (!recorder || state.status !== "recording") return Promise.resolve(null);
      update({ status: "finalizing" });
      stopPromise = new Promise(function (resolve) { resolveStop = resolve; });
      try { recorder.stop(); } catch (error) { fail(error); }
      return stopPromise;
    }

    function discard() {
      if (recorder) {
        recorder.ondataavailable = null;
        recorder.onstop = null;
        recorder.onerror = null;
        try { if (recorder.state !== "inactive") recorder.stop(); } catch (_) {}
      }
      recorder = null;
      chunks = [];
      result = null;
      stopPromise = null;
      resolveStop = null;
      state = resetState();
      emit();
    }

    return {
      start: start,
      stop: stop,
      discard: discard,
      getState: snapshot,
      getRecording: function () { return state.status === "ready" ? result : null; },
    };
  }

  return { createRecordingManager: createRecordingManager };
});
