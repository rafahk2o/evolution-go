(function (root, factory) {
  "use strict";

  var api;
  if (typeof module === "object" && module.exports) {
    api = factory(require("./shared/core.js"), require("./shared/protocol.js"));
    module.exports = api;
  } else {
    api = factory(root.WaCallsCore, root.WaCallsProtocol);
    root.WaCallsController = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function (core, protocol) {
  "use strict";

  function createController(deps) {
    var state = {
      phase: "idle",
      number: "",
      callId: "",
      status: "",
      connectedAt: 0,
      muted: false,
      busy: false,
      error: "",
    };
    var media = null;
    var pollTimer = null;
    var disposed = false;
    var remoteEnded = false;

    function snapshot() { return Object.assign({}, state); }
    function emit() { if (deps.onState) deps.onState(snapshot()); }
    function update(values) { Object.assign(state, values); emit(); }

    async function request(message) {
      var result = await deps.sendMessage(message);
      if (!result || result.ok !== true) throw new Error(result && result.error || "Falha ao controlar a chamada.");
      return result;
    }

    async function setupAudio() {
      var stream = await deps.mediaDevices.getUserMedia({ audio: true, video: false });
      var context = new deps.AudioContext({ latencyHint: "interactive" });
      try {
        await context.audioWorklet.addModule(deps.runtimeGetURL("audio-worklet.js"));
        await context.resume();
        var source = context.createMediaStreamSource(stream);
        var capture = new deps.AudioWorkletNode(context, "wacalls-capture");
        var playback = new deps.AudioWorkletNode(context, "wacalls-playback", { outputChannelCount: [1] });
        var silent = context.createGain();
        silent.gain.value = 0;
        source.connect(capture);
        capture.connect(silent);
        silent.connect(context.destination);
        playback.connect(context.destination);
        return { stream: stream, context: context, source: source, capture: capture, playback: playback, silent: silent };
      } catch (error) {
        stream.getTracks().forEach(function (track) { track.stop(); });
        try { await context.close(); } catch (_) {}
        throw error;
      }
    }

    function disposeAudio(audio) {
      if (!audio) return;
      try { audio.capture.port.postMessage({ enabled: false }); } catch (_) {}
      try { audio.stream.getTracks().forEach(function (track) { track.stop(); }); } catch (_) {}
      [audio.source, audio.capture, audio.playback, audio.silent].forEach(function (node) {
        try { if (node) node.disconnect(); } catch (_) {}
      });
      try { audio.context.close(); } catch (_) {}
    }

    function cleanupMedia() {
      var current = media;
      media = null;
      state.muted = false;
      if (!current) return;
      disposeAudio(current);
      try { current.channel.close(); } catch (_) {}
      try { current.peer.close(); } catch (_) {}
    }

    function waitForIce(peer) {
      if (peer.iceGatheringState === "complete") return Promise.resolve();
      return new Promise(function (resolve) {
        var done = false;
        function finish() {
          if (done) return;
          done = true;
          peer.removeEventListener("icegatheringstatechange", changed);
          resolve();
        }
        function changed() { if (peer.iceGatheringState === "complete") finish(); }
        peer.addEventListener("icegatheringstatechange", changed);
        deps.setTimeout(finish, 8000);
      });
    }

    function waitForChannel(channel) {
      if (channel.readyState === "open") return Promise.resolve();
      return new Promise(function (resolve, reject) {
        var timer = deps.setTimeout(function () { reject(new Error("O canal de áudio WebRTC não abriu.")); }, 12000);
        channel.addEventListener("open", function () { deps.clearTimeout(timer); resolve(); }, { once: true });
        channel.addEventListener("error", function () { deps.clearTimeout(timer); reject(new Error("Falha no canal de áudio WebRTC.")); }, { once: true });
      });
    }

    async function negotiate(callId, audio) {
      var peer = new deps.RTCPeerConnection({});
      var channel = peer.createDataChannel("pcm");
      channel.binaryType = "arraybuffer";
      var current = Object.assign(audio, { callId: callId, peer: peer, channel: channel });
      media = current;
      audio.capture.port.onmessage = function (event) {
        if (media !== current || state.muted || channel.readyState !== "open") return;
        if (channel.bufferedAmount > 512 * 1024) return;
        try { channel.send(event.data); } catch (_) {}
      };
      channel.onmessage = function (event) {
        if (media !== current) return;
        if (event.data instanceof ArrayBuffer) audio.playback.port.postMessage(event.data, [event.data]);
        else if (event.data && typeof event.data.arrayBuffer === "function") {
          event.data.arrayBuffer().then(function (buffer) {
            if (media === current) audio.playback.port.postMessage(buffer, [buffer]);
          });
        }
      };
      var offer = await peer.createOffer();
      await peer.setLocalDescription(offer);
      await waitForIce(peer);
      if (!peer.localDescription || !peer.localDescription.sdp) throw new Error("O navegador não gerou uma oferta WebRTC válida.");
      var answer = await request({
        type: protocol.TYPES.CALL_WEBRTC,
        callId: callId,
        sdpOffer: peer.localDescription.sdp,
      });
      await peer.setRemoteDescription({ type: "answer", sdp: answer.sdpAnswer });
      await waitForChannel(channel);
    }

    function stopPolling() {
      if (pollTimer != null) deps.clearInterval(pollTimer);
      pollTimer = null;
    }

    async function poll() {
      if (!state.callId || disposed) return;
      try {
        var result = await request({ type: protocol.TYPES.CALL_ACTIVE });
        var call = result.calls.find(function (item) { return item.callId === state.callId; });
        if (!call) {
          stopPolling();
          cleanupMedia();
          remoteEnded = true;
          update({ phase: "idle", status: "ended", busy: false });
          return;
        }
        var values = { status: call.status || state.status };
        if (call.status === "connected" && !state.connectedAt) values.connectedAt = deps.now();
        update(values);
        if (core.isTerminalStatus(call.status)) {
          stopPolling();
          cleanupMedia();
          remoteEnded = true;
          update({ phase: call.status === "failed" ? "failed" : "idle", busy: false });
        }
      } catch (error) {
        update({ error: error.message });
      }
    }

    function startPolling() {
      stopPolling();
      pollTimer = deps.setInterval(poll, 1500);
    }

    async function start(rawNumber) {
      var number = core.normalizePhone(rawNumber);
      if (state.busy || (state.callId && !remoteEnded)) throw new Error("Já existe uma chamada em andamento.");
      disposed = false;
      remoteEnded = false;
      update({ phase: "preparing", number: number, callId: "", status: "starting", connectedAt: 0, muted: false, busy: true, error: "" });
      var audio = null;
      try {
        audio = await setupAudio();
        var started = await request({ type: protocol.TYPES.CALL_START, number: number });
        state.callId = started.callId;
        state.status = started.status || "starting";
        emit();
        var negotiatingAudio = audio;
        audio = null;
        await negotiate(started.callId, negotiatingAudio);
        update({ phase: "active", busy: false });
        startPolling();
        return snapshot();
      } catch (error) {
        if (audio) disposeAudio(audio);
        cleanupMedia();
        if (state.callId && !remoteEnded) {
          try { await request({ type: protocol.TYPES.CALL_END, callId: state.callId }); } catch (_) {}
          remoteEnded = true;
        }
        update({ phase: "failed", busy: false, error: error.name === "NotAllowedError" ? "Permissão do microfone negada." : error.message });
        throw new Error(state.error);
      }
    }

    function toggleMute() {
      if (!media) return state.muted;
      state.muted = !state.muted;
      media.capture.port.postMessage({ enabled: !state.muted });
      media.stream.getAudioTracks().forEach(function (track) { track.enabled = !state.muted; });
      emit();
      return state.muted;
    }

    async function end() {
      if (!state.callId) { cleanupMedia(); return snapshot(); }
      update({ phase: "ending", busy: true, error: "" });
      try {
        if (!remoteEnded) await request({ type: protocol.TYPES.CALL_END, callId: state.callId });
        remoteEnded = true;
        update({ phase: "idle", status: "ended", busy: false });
      } catch (error) {
        update({ phase: "failed", busy: false, error: error.message });
        throw error;
      } finally {
        stopPolling();
        cleanupMedia();
      }
      return snapshot();
    }

    async function dispose(options) {
      options = options || {};
      disposed = true;
      stopPolling();
      if (options.endRemote && state.callId && !remoteEnded) {
        try { await deps.sendMessage({ type: protocol.TYPES.CALL_END, callId: state.callId }); } catch (_) {}
        remoteEnded = true;
      }
      cleanupMedia();
    }

    return { start: start, toggleMute: toggleMute, end: end, dispose: dispose, getState: snapshot };
  }

  return { createController: createController };
});
