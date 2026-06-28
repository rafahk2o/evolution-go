# WACalls Browser Local Recording Design

## Goal

Add automatic, local recording to every outbound call made by the standalone
WACalls browser extension. Recording starts when the WebRTC PCM channel opens,
captures both sides of the conversation, and produces an optional WebM/Opus
download after the call ends.

No audio is uploaded to Evolution GO or another server. If the user does not
download the recording, it is discarded when another call starts or the
extension window closes.

## Scope

This release includes:

- automatic recording of local microphone and remote call audio;
- a visible recording indicator during the call;
- WebM/Opus output with a WebM fallback;
- an optional download action after local or remote termination;
- deterministic finalization on hang-up, remote termination, failure, and the
  in-memory size limit;
- discard on the next call or window closure;
- a clear recording-consent notice in the extension UI;
- automated tests and regenerated unpacked/ZIP artifacts.

It does not include MP4 transcoding, recording upload, server-side storage,
recording history, persistence across extension-window closure, pause/resume,
or incoming calls.

## Architecture

A new focused recording unit manages `MediaRecorder`, chunks, output metadata,
and finalization. It is owned by the existing call controller and depends only
on injected browser primitives, making it testable without a real microphone.

The controller remains responsible for the full call lifecycle. It starts the
recorder after the `pcm` data channel opens, stops it before browser audio
resources are released, exposes recording state to the window, and retains at
most one completed Blob in memory.

The window owns the download action. It receives the Blob and filename from the
controller only after an explicit click, creates a temporary object URL, clicks
a local anchor with the `download` attribute, and revokes the URL. This does not
require the Chrome `downloads` permission.

## Audio Graph

The existing Web Audio graph is extended with one
`MediaStreamAudioDestinationNode`:

```text
microphone source ----> capture worklet ----> silent gain ----> speakers
         |                                      
         +-------------------------------> recording destination

remote PCM ----> playback worklet -------> speakers
                        |
                        +-----------------> recording destination
```

The microphone source supplies the local side to the recording destination.
The playback worklet supplies decoded remote PCM. Existing speaker playback and
data-channel transport remain unchanged.

Muting continues to disable the microphone track and capture worklet. The
recording therefore contains silence for the local side while mute is active,
but continues to capture the remote side.

## Format Selection

Before recording, the extension selects the first supported MIME type in this
order:

1. `audio/webm;codecs=opus`;
2. `audio/webm`.

If neither is supported, the call continues normally and the UI reports that
recording is unavailable. MP4 is not attempted because Chrome and Edge do not
provide a reliable audio-only MP4 `MediaRecorder` path.

The recorder starts with a one-second timeslice so chunks are delivered
incrementally. Chunks with zero bytes are ignored. The recording unit maintains
a running byte count without repeatedly rebuilding a Blob.

## Recording Lifecycle

At the start of a new call, any prior, undownloaded recording is discarded.

After WebRTC negotiation and opening of the `pcm` channel:

1. Create the mixed recording destination.
2. Connect local and remote audio nodes to it.
3. Select a supported WebM MIME type.
4. Construct `MediaRecorder` with the destination stream.
5. Start it with a one-second timeslice.
6. Publish `recording` state to the window.

Recording finalizes on:

- local hang-up;
- remote termination or disappearance from `/call/active`;
- terminal call failure after media has opened;
- extension cleanup while the window is still able to finalize;
- reaching 250 MiB of collected chunks.

Finalization is idempotent. It requests recorder stop once, waits for the final
`dataavailable` and `stop` events, creates one Blob, disconnects recording-only
nodes, and publishes `ready` state. Ordinary call cleanup happens only after
this attempt completes.

If the extension window is closed, any in-memory recording and object URL are
discarded. Closing during a call still performs the existing best-effort remote
hang-up; no download is forced.

## State and Filename

The public call-controller state adds:

```json
{
  "recordingStatus": "inactive|recording|finalizing|ready|unavailable|failed",
  "recordingBytes": 0,
  "recordingAvailable": false,
  "recordingFilename": ""
}
```

The Blob itself is not placed in render state or extension storage. The
controller exposes a narrow `getRecording()` method returning the current Blob
and filename only when `recordingStatus` is `ready`.

Filenames use:

```text
evolution-call-<digits-only-phone>-<UTC-YYYYMMDD-HHmmss>.webm
```

Only normalized phone digits and an internally generated UTC timestamp enter
the filename.

## User Interface

During an active recording, the call panel shows a red pulse and `Gravando`.
There is no pause or manual stop button because recording is automatic for every
call.

After a call ends, the dialer displays a recording card with:

- `Gravação pronta`;
- formatted file size;
- `Baixar gravação` button;
- a short explanation that starting another call or closing the window discards
  an undownloaded recording.

Clicking download does not delete the in-memory recording immediately, allowing
the user to retry the download until another call starts or the window closes.

The configuration/dialer area also displays a concise notice that recording is
automatic and the user is responsible for complying with applicable consent
and privacy requirements.

## Failure Handling

- Missing `MediaRecorder` support sets `unavailable` and does not fail the call.
- Unsupported WebM MIME types set `unavailable` and do not fail the call.
- Recorder construction or start failure sets `failed`, disconnects recording
  nodes, and leaves the call and browser audio active.
- Recorder runtime errors finalize or discard partial data safely and expose a
  concise Portuguese message without terminating the call.
- The 250 MiB limit stops the recorder, retains the completed Blob for optional
  download, and displays that the limit was reached.
- Empty recordings are discarded and do not show a download button.
- Repeated terminal events cannot stop or download the same recorder twice.
- Blob contents, raw PCM, and object URLs are never logged.

## Security and Privacy

Recording is entirely local to the extension window. It does not add API routes,
runtime-message operations, host permissions, or server changes. Audio is never
placed in `chrome.storage.local`, synchronized storage, logs, or network
requests.

The visible recording indicator and consent notice are mandatory. They inform
the extension user but do not determine whether recording is legal; the user is
responsible for obtaining any consent required in their jurisdiction.

## Testing

Recording-unit tests use fake media streams and `MediaRecorder` to cover:

- MIME preference and fallback;
- one-second timeslice startup;
- non-empty chunk collection and byte accounting;
- idempotent finalization and final chunk handling;
- empty recording discard;
- 250 MiB limit finalization;
- unsupported and runtime-error states.

Controller tests cover:

- both local source and remote playback connected to the recording destination;
- automatic start only after the PCM channel opens;
- mute preserving remote recording while silencing local capture;
- finalization before audio cleanup on local, remote, failed, and disposed paths;
- prior recording discard before another call;
- narrow Blob access only in `ready` state.

Window tests cover:

- visible recording indicator;
- post-call recording card and formatted size;
- explicit anchor download with the expected filename;
- object URL revocation;
- discarded recording on next call/window closure;
- recording-consent notice.

Full verification runs all extension tests, static validation, packaging, and
artifact secret/coupling scans. Manual verification performs a real call, checks
both voices in the downloaded WebM, tests mute, remote/local hang-up, download,
and discard behavior in current Chrome and Edge.

## Acceptance Criteria

- Every call with an opened PCM channel starts local recording automatically.
- A visible indicator remains present while recording.
- The resulting WebM contains both local and remote voices.
- Local mute records local silence while remote audio continues.
- Local hang-up, remote termination, and call failure finalize at most one Blob.
- A completed non-empty recording can be downloaded through an explicit user
  action with the defined filename.
- Not downloading never writes the recording to disk or storage.
- Starting another call or closing the window discards the prior recording.
- Recording incompatibility or failure never prevents calling, muting, or
  hanging up.
- The unpacked `dist/` folder and versioned ZIP are regenerated and validated.

