# Standalone WACalls Browser Extension Design

## Goal

Build an independent Chrome and Microsoft Edge Manifest V3 extension for
outbound WhatsApp voice calls through the existing Evolution GO WACalls API.
The extension has no Chatwoot integration. A user configures one Evolution API
URL and API key once, enters a destination phone number for each call, and uses
a compact floating browser window to call, mute, and hang up.

## Scope

The first release includes:

- Chrome and Microsoft Edge desktop;
- one locally stored Evolution API configuration;
- configuration verification before use;
- outbound calls only;
- bidirectional browser audio;
- microphone mute and unmute;
- active-call status and elapsed duration;
- call termination and deterministic resource cleanup;
- unpacked extension and ZIP packaging.

Incoming calls, call recording, call history, multiple profiles, Firefox,
mobile browsers, store publishing, contact lists, and Chatwoot integration are
outside this release.

## Architecture

The extension uses Manifest V3 and contains four isolated units:

1. A service worker opens or focuses the call window, stores credentials, and
   performs an allowlisted set of authenticated Evolution API requests.
2. A compact extension window owns the user interface, microphone, AudioContext,
   AudioWorklet, RTCPeerConnection, PCM data channel, timers, and call lifecycle.
   It handles the API key only while the user is submitting configuration.
3. Pure shared modules normalize and validate URLs, phone numbers, messages,
   call responses, and statuses.
4. A packaged AudioWorklet converts microphone and playback audio to and from
   signed 16-bit little-endian mono PCM at 16 kHz, matching Evolution GO.

Clicking the toolbar icon creates or focuses one extension window with
`chrome.windows.create({ type: "popup" })`. The window remains open until the
user closes it, but the browser does not guarantee that it stays above every
other application window.

The service worker may suspend between requests. Durable configuration is kept
in `chrome.storage.local`, and every message handler reconstructs its state from
storage. Media and live-call state remain in the visible extension window; the
call is ended when that window closes.

## Manifest and Permissions

The extension requests only:

- `storage` for local configuration;
- permission for the exact configured Evolution origin;
- the browser capabilities required to open and focus the extension window.

The configured API URL must use HTTPS. Plain HTTP is accepted only for
`localhost` or `127.0.0.1` development origins. The extension requests
the exact origin permission after an explicit save/test action and does not use
permanent `<all_urls>` access, content scripts, externally connectable messages,
or remote JavaScript.

## Configuration and Credential Boundary

The configuration model is:

```json
{
  "apiUrl": "https://evolution.example",
  "apiKey": "instance-secret",
  "instanceName": "verified-instance-name",
  "connected": true,
  "loggedIn": true,
  "lastVerifiedAt": "2026-06-28T12:00:00Z"
}
```

Saving configuration normalizes the origin, requests permission for that exact
origin, and calls `GET /instance/status` through the service worker. Only a
successful response reporting both `Connected` and `LoggedIn` is persisted as
verified. The optional `Name` value is retained as the display name. Changing
the URL or API key invalidates the previous verification.

The configuration window necessarily sends the API key once in the typed save
message, then clears the input. The service worker adds the `apikey` header
internally for verification and all later calls. Responses and non-configuration
messages omit the API key and full upstream response bodies. The key is not
stored in synchronized storage, DOM attributes, URL parameters, page storage, or
logs. A local machine administrator or user inspecting the extension's own
storage can recover it; protection from the device owner is not a security
objective.

## Service Worker API Boundary

The worker accepts typed internal messages for only these operations:

- save and verify configuration with `GET /instance/status`;
- read sanitized configuration state;
- start a call with `POST /call/start`;
- negotiate WebRTC with `POST /call/:callId/webrtc`;
- read active state with `GET /call/active`;
- end a call with `DELETE /call/:callId`.

Every call operation uses a stable random browser client ID in
`X-Call-Client-ID`. The worker derives the request URL and headers from verified
storage. The window cannot supply an arbitrary URL, HTTP method, header, or API
path. Message validation limits number, call ID, SDP, and response sizes.

## User Interface

The compact window has two main states.

### Initial configuration

The user enters:

- Evolution API URL;
- API key;
- `Salvar e testar` action.

The UI shows testing, verified/connected, disconnected, invalid credential, and
network failure states. The API key input is masked. A settings action remains
available after successful setup.

### Dialer and active call

The idle dialer shows a phone number field requiring country and area codes and
a `Ligar` action. During a call it shows:

- normalized destination;
- preparing, ringing, connected, ending, ended, or failed status;
- elapsed duration after connection;
- mute/unmute control;
- hang-up control.

Only one non-terminal call can exist in the window. Configuration cannot be
changed while a call is active.

## Call and Media Flow

1. Normalize the destination to digits and validate its length.
2. Request microphone permission and initialize browser audio resources.
3. Ask the service worker to call `POST /call/start`.
4. Create an `RTCPeerConnection` and a data channel named `pcm`.
5. Create an SDP offer and wait for ICE gathering to finish within a bounded
   timeout.
6. Send the offer through the service worker to `/call/:callId/webrtc`.
7. Apply the SDP answer and wait for the data channel to open.
8. Send captured PCM and enqueue received PCM for playback.
9. Poll `/call/active` while the call is non-terminal and update status.
10. On hang-up, call `DELETE /call/:callId` and release all browser resources.

Mute disables both the microphone track and the capture worklet. Capture frames
are dropped while the data channel buffered amount is above the established safe
limit. The audio implementation reuses the proven framing and lifecycle
behavior of the existing Evolution Manager WACalls widget without depending on
the Manager page.

## Failure Handling and Cleanup

- Invalid URL, API key, or instance status leaves the configuration unverified.
- A missing exact-origin permission blocks API access and explains how to grant
  it.
- An invalid phone number is rejected before microphone or network work.
- Microphone denial does not call `/call/start`.
- If call creation succeeds but media negotiation fails, the extension makes a
  best-effort termination request.
- Upstream `401`, disconnected instance, ownership conflict, network failure,
  and negotiation timeout are translated to concise Portuguese messages.
- Closing the call window during an active call triggers best-effort hang-up.
- Every terminal path clears polling and duration timers, stops microphone
  tracks, disconnects audio nodes, closes AudioContext, data channel, and peer
  connection, and releases in-memory call state.
- API keys, SDP, PCM frames, and full upstream response bodies are excluded from
  logs.

## Packaging

All implementation lives under `extensions/wacalls-browser/`. The package is
self-contained and dependency-free at runtime. Build scripts validate manifest
references, copy only runtime assets to `dist/`, and create a versioned ZIP that
can be loaded in Chrome or Edge developer mode.

The package contains no real API URL, API key, phone number, or environment data.

## Testing

Pure unit tests cover:

- API origin normalization and HTTP development exceptions;
- phone normalization and rejection;
- typed message and payload validation;
- call status normalization and terminal-state detection;
- sanitized configuration responses.

Service worker tests mock Chrome storage, permissions, windows, and fetch to
prove that:

- the API key never appears in responses to the window;
- requests target only the verified origin and allowlisted routes;
- caller-supplied URLs, methods, headers, and malformed call IDs are rejected;
- toolbar clicks create or focus only one call window;
- configuration changes require new verification.

Window tests use browser API, media, audio, and WebRTC fakes to prove that:

- microphone access happens before call creation;
- a valid call negotiates a `pcm` data channel;
- mute affects microphone capture;
- status polling updates the interface;
- hang-up, failures, and window closure release every resource;
- a negotiation failure after call creation attempts remote termination.

Static validation checks Manifest V3, local-only scripts, referenced files,
minimal permissions, and absence of Chatwoot content scripts or permissions.
Manual verification loads the unpacked build in current Chrome and Edge and
performs a real outbound call with bidirectional audio.

## Acceptance Criteria

- Chrome and Edge load the unpacked extension without manifest errors.
- Clicking the toolbar icon opens or focuses one compact floating call window.
- The user saves and verifies one Evolution API URL and API key locally.
- Subsequent use opens the dialer with only the phone number left to enter.
- A valid number starts an outbound WhatsApp call with bidirectional audio.
- The user can mute, unmute, and end the call and can see status and duration.
- Closing the window or encountering a failure releases local resources and
  attempts to terminate any created remote call.
- The API key remains confined to trusted extension storage and the service
  worker's authenticated request path.
- The worker cannot be used as a general authenticated HTTP proxy.
- No Chatwoot code, route, cookie, DOM, permission, or server change is used.
