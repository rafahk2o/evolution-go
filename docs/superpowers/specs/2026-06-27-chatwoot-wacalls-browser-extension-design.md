# Chatwoot WACalls Browser Extension Design

## Status

Approved design. This specification supersedes the native Chatwoot temporary
session broker design and its implementation plan.

## Goal

Build a Chrome and Microsoft Edge Manifest V3 extension that adds an outbound
WhatsApp call button to Chatwoot conversations. Each user configures one or more
Evolution GO profiles using an API URL and instance API key. The extension maps
the selected Chatwoot inbox to the correct profile automatically and uses the
existing WACalls WebRTC API without changing or rebuilding Chatwoot.

## Scope

The first release supports:

- Chrome and Edge desktop;
- multiple Evolution profiles per browser;
- automatic Chatwoot inbox-to-instance mapping;
- manual mapping fallback;
- outbound calls;
- microphone mute/unmute;
- call termination;
- active-call status polling;
- packaging as an unpacked extension and ZIP archive.

Incoming-call notification and answering, Chrome Web Store publishing, Firefox,
mobile browsers, call recording, and centralized credential management are not
part of this release.

## Architecture

The extension has four isolated units:

1. The popup/options UI manages profiles and Chatwoot site activation.
2. The Manifest V3 service worker owns Evolution credentials and performs an
   allowlisted set of Evolution API requests.
3. A content script reads the current Chatwoot context, injects the call button
   and modal, and owns browser media/WebRTC resources.
4. Pure shared modules normalize URLs, parse Chatwoot routes/webhooks, map
   profiles, validate messages, and format errors.

The Chatwoot page cannot read extension JavaScript variables. The service worker
sets the `chrome.storage.local` access level to trusted extension contexts, so
content scripts receive only non-secret profile metadata. No externally
connectable messaging API is exposed.

## Manifest and Permissions

The extension uses Manifest V3 with:

- `storage` for local profiles;
- `scripting` for dynamically registered content scripts;
- `activeTab` for explicit activation of the current Chatwoot site;
- `optional_host_permissions` covering HTTP and HTTPS origins discovered at
  runtime;
- web-accessible audio worklet resources restricted to activated Chatwoot
  origins.

The extension requests exact Chatwoot and Evolution origins only after a user
action. It does not receive permanent access to every site during installation.
Remote JavaScript and remote code evaluation are prohibited.

## Profile Model

Profiles are stored only in `chrome.storage.local`:

```json
{
  "id": "generated-uuid",
  "label": "Suporte",
  "apiUrl": "https://evogo.example",
  "apiKey": "instance-secret",
  "instanceId": "instance-uuid",
  "instanceName": "support",
  "connected": true,
  "manualInboxMappings": [
    {
      "chatwootOrigin": "https://chat.example",
      "accountId": "1",
      "inboxId": "12"
    }
  ],
  "lastVerifiedAt": "2026-06-27T21:00:00Z"
}
```

The popup never trusts a user-entered profile without verification. Saving or
testing a profile causes the service worker to request permission for the exact
Evolution origin and call `/instance/status` with the API key. The returned
instance ID/name and connection state replace user-supplied identity fields.

The API key is masked in UI, omitted from logs and runtime messages, and never
stored in `storage.sync`, page storage, Chatwoot storage, DOM attributes, or
content-script state. A local machine administrator or a user inspecting the
extension's own storage can still recover it; the extension does not claim to
protect credentials from the device owner.

## Chatwoot Site Activation

While the user is on a Chatwoot dashboard, the popup offers "Ativar neste
Chatwoot". After the user grants the exact origin permission, the service worker
dynamically registers the content script for that origin and stores a non-secret
site record.

Site activation verifies recognizable Chatwoot dashboard structure and routes.
The extension does not run call injection on login pages, public widgets, or
unrelated applications hosted on the same origin.

## Chatwoot Context Discovery

The content script watches SPA navigation and parses supported conversation
routes to obtain account and conversation display IDs. It reads the existing
`cw_d_session_info` cookie in memory and uses those headers for same-origin,
read-only Chatwoot API requests. The value is not persisted or sent to the
service worker.

The extension loads:

- the selected conversation to obtain inbox and contact details;
- the account inbox list or selected inbox to obtain channel type and
  `webhook_url`.

Only `Channel::Api` inboxes with an HTTPS webhook matching this route are
eligible for automatic mapping:

```text
/webhooks/chatwoot/:instance/:token
```

The webhook token is discarded immediately. It is never stored or used as an
Evolution credential.

## Automatic Profile Mapping

Mapping compares:

1. normalized webhook origin with the verified profile API origin; and
2. decoded webhook instance segment with the verified profile instance name.

Exactly one match activates the call button. Zero matches leave the conversation
without an active button and the popup explains that no profile is mapped.
Multiple matches are treated as ambiguous and require a manual selection.

A manual mapping binds Chatwoot origin, account ID, and inbox ID to a profile.
Manual mappings override automatic matches and are removed when their profile is
deleted.

## Service Worker API Boundary

The service worker accepts typed messages from extension content scripts. It is
not a general HTTP proxy. Supported operations are:

- verify profile using `GET /instance/status`;
- start call using `POST /call/start`;
- negotiate media using `POST /call/:callId/webrtc`;
- read active state using `GET /call/active`;
- end call using `DELETE /call/:callId`.

Every call message contains a profile ID and stable browser client ID. The
service worker loads the secret profile internally and adds `apikey` and
`X-Call-Client-ID`. It validates operation names, call ID shape, HTTP method,
payload size, and destination number before making a request. The content script
cannot select an arbitrary URL or header.

The service worker may suspend between requests. All durable configuration is in
storage and every handler can reconstruct its required state independently.

## User Interface

### Popup and options

The popup provides:

- profile list with verified/connected/error state;
- add, edit, test, and remove actions;
- URL, label, and API key fields;
- current Chatwoot site activation status;
- detected account/inbox/profile mapping;
- manual mapping fallback;
- concise permission and credential warnings.

### Conversation button

The content script inserts one accessible telephone button into the Chatwoot
conversation header action area. It uses a unique extension data attribute and
checks for an existing element before every injection. A bounded
`MutationObserver` and route-change watcher handle Chatwoot SPA rendering without
creating duplicates.

The button is shown only for an eligible, uniquely mapped inbox with a usable
contact phone number. Disconnected instances show a disabled button with an
explanation.

### Call modal

The extension renders its modal in a Shadow DOM root to isolate styles from
Chatwoot. The modal displays:

- profile/instance label;
- normalized destination, editable before dialing;
- preparation, ringing, connected, ending, ended, and failure states;
- elapsed duration;
- call, mute/unmute, and hang-up controls;
- actionable Portuguese error messages.

## Browser Media and WebRTC

The content script requests microphone access before starting an outgoing call.
It loads a packaged, web-accessible AudioWorklet module and creates capture and
playback worklets using the same 16 kHz mono PCM framing used by the existing
Evolution Manager widget.

Call flow:

1. Validate and normalize the destination.
2. Obtain microphone permission and initialize audio nodes.
3. Ask the service worker to call `POST /call/start`.
4. Create an `RTCPeerConnection` and `pcm` data channel.
5. Create an SDP offer and wait for ICE gathering.
6. Send the offer through the service worker to `/call/:id/webrtc`.
7. Apply the SDP answer and wait for the data channel to open.
8. Send captured PCM and enqueue received PCM for playback.
9. Poll `/call/active` while the modal is open.
10. End the call and release all resources on hang-up, failure, navigation, tab
    closure, extension update, or modal close confirmation.

Mute disables both the microphone track and capture worklet. Backpressure drops
capture frames when the data channel buffered amount exceeds the existing safe
limit.

## Failure Handling

- Invalid or unreachable Evolution URL leaves a profile unsaved/unverified.
- A `401` marks the profile credential invalid without exposing response data.
- Permission denial explains which origin must be granted.
- Missing/expired Chatwoot authentication hides the button and asks the user to
  sign in again.
- Unsupported Chatwoot routes or API responses fail closed without DOM changes.
- Missing or ambiguous mapping is reported in the popup.
- Microphone denial does not call `/call/start`.
- If call creation succeeds but WebRTC setup fails, the extension sends a
  best-effort DELETE before cleanup.
- Navigation and extension lifecycle events always stop tracks, disconnect audio
  nodes, close AudioContext, data channel and peer connection, and clear timers.
- Secrets, auth cookies, SDP, PCM, and full upstream response bodies are not
  logged.

## Packaging

The extension is self-contained under `extensions/chatwoot-wacalls/`. Production
artifacts contain no remote dependencies or source-map secrets. A build script:

1. runs unit tests and static manifest validation;
2. copies only runtime files to `dist/`;
3. creates a versioned ZIP for Chrome/Edge;
4. prints instructions for "Load unpacked" and Edge's equivalent.

The repository does not include any real URL, API key, Chatwoot auth cookie, or
user profile in the package.

## Testing

Pure unit tests cover:

- API URL normalization;
- Chatwoot route and webhook parsing;
- automatic and manual profile mapping;
- destination normalization;
- operation/message allowlisting;
- Evolution error translation;
- status normalization and terminal states.

Service worker tests mock Chrome storage, permissions and fetch to prove that:

- credentials never appear in responses to content scripts;
- requests target only a verified profile origin and allowed call route;
- arbitrary paths, methods, headers, oversized SDP, and invalid call IDs fail;
- profile verification and deletion update permissions/state correctly.

Content-script tests use a DOM fixture to prove:

- one button is injected and reinjection is idempotent;
- ineligible or ambiguous inboxes do not receive a button;
- conversation navigation refreshes context;
- modal and Shadow DOM lifecycle work;
- microphone is acquired before call start;
- WebRTC failure and hang-up release all resources.

Manual verification loads the unpacked extension in current Chrome and Edge,
activates a test Chatwoot origin, configures multiple test profiles, verifies
automatic mapping, and performs an outbound call against the deployed Evolution
GO API.

## Acceptance Criteria

- Chrome and Edge load the unpacked extension without manifest errors.
- A user can save and verify multiple Evolution URL/API-key profiles.
- A configured Chatwoot conversation automatically selects the profile matching
  its API inbox webhook.
- Exactly one button appears in the conversation header after SPA navigation and
  rerenders.
- The contact phone is prefilled and an outbound call can be started, muted, and
  ended with bidirectional audio.
- Evolution API keys never enter the Chatwoot page, content-script state,
  extension messages, logs, or synchronized storage.
- The service worker cannot be used for arbitrary authenticated Evolution API
  requests.
- Failures release microphone and WebRTC resources and do not leave active calls
  when a best-effort termination is possible.
- No Chatwoot or Evolution server code change is required for this extension.
