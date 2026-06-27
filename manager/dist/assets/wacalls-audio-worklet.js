"use strict";

class WaCallsCaptureProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.pending = [];
    this.enabled = true;
    this.inputFrameSize = Math.max(1, Math.round(sampleRate * 0.02));
    this.port.onmessage = (event) => {
      if (event && event.data && typeof event.data.enabled === "boolean") {
        this.enabled = event.data.enabled;
      }
    };
  }

  process(inputs) {
    const input = inputs[0] && inputs[0][0];
    if (!input || !input.length) return true;

    for (let index = 0; index < input.length; index += 1) {
      this.pending.push(input[index]);
    }

    while (this.pending.length >= this.inputFrameSize) {
      const source = this.pending.splice(0, this.inputFrameSize);
      if (!this.enabled) continue;

      const pcm = new Int16Array(320);
      const ratio = source.length / pcm.length;
      for (let index = 0; index < pcm.length; index += 1) {
        const position = index * ratio;
        const left = Math.floor(position);
        const right = Math.min(left + 1, source.length - 1);
        const fraction = position - left;
        const sample = source[left] + (source[right] - source[left]) * fraction;
        const clamped = Math.max(-1, Math.min(1, sample));
        pcm[index] = clamped < 0 ? clamped * 32768 : clamped * 32767;
      }
      this.port.postMessage(pcm.buffer, [pcm.buffer]);
    }

    return true;
  }
}

class WaCallsPlaybackProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = new Float32Array(Math.max(2048, Math.round(sampleRate * 2)));
    this.readIndex = 0;
    this.writeIndex = 0;
    this.available = 0;
    this.port.onmessage = (event) => this.enqueue(event && event.data);
  }

  enqueue(value) {
    const raw = value && value.pcm ? value.pcm : value;
    if (!(raw instanceof ArrayBuffer) || raw.byteLength < 2) return;

    const pcm = new Int16Array(raw);
    const outputLength = Math.max(1, Math.round(pcm.length * sampleRate / 16000));
    for (let index = 0; index < outputLength; index += 1) {
      const position = index * 16000 / sampleRate;
      const left = Math.min(Math.floor(position), pcm.length - 1);
      const right = Math.min(left + 1, pcm.length - 1);
      const fraction = position - left;
      const sample = pcm[left] + (pcm[right] - pcm[left]) * fraction;
      this.write(sample / 32768);
    }
  }

  write(sample) {
    if (this.available === this.buffer.length) {
      this.readIndex = (this.readIndex + 1) % this.buffer.length;
      this.available -= 1;
    }
    this.buffer[this.writeIndex] = sample;
    this.writeIndex = (this.writeIndex + 1) % this.buffer.length;
    this.available += 1;
  }

  read() {
    if (!this.available) return 0;
    const sample = this.buffer[this.readIndex];
    this.readIndex = (this.readIndex + 1) % this.buffer.length;
    this.available -= 1;
    return sample;
  }

  process(_inputs, outputs) {
    const output = outputs[0] || [];
    if (!output.length) return true;
    for (let index = 0; index < output[0].length; index += 1) {
      const sample = this.read();
      for (let channel = 0; channel < output.length; channel += 1) {
        output[channel][index] = sample;
      }
    }
    return true;
  }
}

registerProcessor("wacalls-capture", WaCallsCaptureProcessor);
registerProcessor("wacalls-playback", WaCallsPlaybackProcessor);
