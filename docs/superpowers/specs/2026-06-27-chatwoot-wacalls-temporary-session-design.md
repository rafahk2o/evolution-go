# Chatwoot WACalls Temporary Session Design

## Goal

Add a native outbound WhatsApp call button to Chatwoot conversations backed by
Evolution GO API inboxes. The browser must use the existing WACalls WebRTC
transport without receiving an Evolution instance API key.

The first release supports starting, muting, and ending outbound calls. Incoming
call notification and answering are intentionally outside this scope.

## Constraints

- Evolution GO is publicly available at `https://evogo.melck.app`.
- Chatwoot and Evolution GO are self-hosted and their server environments can be
  configured with the same secret.
- The Chatwoot API inbox webhook already identifies the Evolution origin and
  instance.
- Existing Manager call clients that authenticate with `apikey` must continue to
  work unchanged.
- The existing Chatwoot Twilio voice implementation must remain independent from
  WACalls.
- Browser microphone access requires the Chatwoot frontend to run over HTTPS.

## Architecture

The Chatwoot browser requests a temporary call session from an authenticated,
same-origin Chatwoot endpoint. Chatwoot validates the account, agent,
conversation, and inbox from server-side records. It extracts the Evolution
origin and instance name from the API channel webhook URL, signs a canonical
request with a shared server secret, and sends it to Evolution GO.

Evolution GO validates the signature, replay protections, and the instance's
stored Chatwoot settings. It returns an opaque Bearer token valid for ten
minutes. The browser then talks directly to Evolution GO for call signaling and
WebRTC negotiation. PCM audio continues over the browser-to-Evolution WebRTC
data channel.

The browser silently requests a replacement token before expiry. A refreshed
token remains bound to the same instance and `X-Call-Client-ID`, so a long call
can still be controlled after the original token expires.

## Shared Configuration

Both servers receive the same value:

```env
CHATWOOT_CALL_SESSION_SECRET=<at least 32 random bytes>
```

The value is never serialized to the frontend, returned by an API, or written to
logs. If it is absent, only temporary session issuance is disabled; normal
Evolution and Chatwoot functionality remains available.

## Chatwoot Session Broker

Chatwoot exposes:

```text
POST /api/v1/accounts/:account_id/evolution_calls/session
```

Request body:

```json
{
  "inbox_id": 12,
  "conversation_id": 345,
  "client_id": "chatwoot-agent-uuid"
}
```

The controller derives `agent_id` from the authenticated Chatwoot user rather
than accepting it from the request. It loads the conversation and inbox through
the current account, confirms that the conversation belongs to the inbox, and
requires a `Channel::Api` webhook URL matching this shape:

```text
https://evolution.example/webhooks/chatwoot/:instance/:webhook_token
```

Only the HTTPS origin and instance name are used for the session request. The
webhook token is not sent to the browser and is not used as call authorization.

Chatwoot sends the following signed JSON to the Evolution origin:

```text
POST /call/session
```

```json
{
  "chatwootUrl": "https://chatwoot.example",
  "accountId": "1",
  "inboxId": "12",
  "conversationId": "345",
  "agentId": "27",
  "clientId": "chatwoot-agent-uuid",
  "instanceName": "support"
}
```

The signature headers are:

```text
X-Chatwoot-Call-Timestamp: <unix seconds>
X-Chatwoot-Call-Nonce: <cryptographically random value>
X-Chatwoot-Call-Signature: <lowercase hex HMAC-SHA256>
```

The signed bytes are exactly:

```text
<timestamp>\n<nonce>\n<raw request body>
```

The Chatwoot endpoint returns only the Evolution origin, temporary token,
expiry, instance display name, and data required by the call client.

## Evolution Session Issuer

`POST /call/session` does not use the normal API-key middleware. Its handler:

1. Rejects requests when `CHATWOOT_CALL_SESSION_SECRET` is missing.
2. Limits the request body and validates required fields.
3. Requires a timestamp within 60 seconds of server time.
4. Rejects a nonce already seen during the replay window.
5. Compares the HMAC signature in constant time.
6. Resolves the instance by name.
7. Requires Chatwoot integration to be enabled.
8. Compares normalized Chatwoot origin, account ID, and inbox ID with the stored
   instance Chatwoot settings.
9. Applies a bounded per-instance and per-agent issuance rate.
10. Issues a 256-bit random opaque token.

Only a SHA-256 hash of the token is retained in memory. A session record holds:

- instance ID;
- Chatwoot account, inbox, conversation, and agent IDs;
- required `X-Call-Client-ID`;
- issue and expiry timestamps.

Sessions expire after ten minutes and are periodically removed. Evolution
restart invalidates all temporary sessions by design.

Successful response:

```json
{
  "accessToken": "opaque-value",
  "tokenType": "Bearer",
  "expiresAt": "2026-06-28T00:10:00Z",
  "instanceId": "instance-uuid",
  "instanceName": "support"
}
```

## Call Authentication

The `/call/*` middleware accepts either:

- the existing `apikey` instance token; or
- `Authorization: Bearer <temporary-token>`.

Bearer authentication validates token expiry and requires the request's
`X-Call-Client-ID` to exactly match the session. It loads the session's instance
and places the same instance and company values in the request context as the
existing middleware.

The Bearer token is valid only for call routes. It cannot access instance,
message, contact, company, or administrative endpoints. Existing Manager
behavior and API-key authentication are unchanged.

## Chatwoot User Interface

`ConversationHeader.vue` renders an Evolution call button before the existing
conversation actions when all conditions are true:

- the inbox is `Channel::Api`;
- its webhook URL is a valid Evolution Chatwoot webhook;
- the current contact has a usable phone number;
- the conversation belongs to that inbox.

The UI consists of isolated components:

- `EvolutionCallButton.vue` controls visibility and opens the dialog;
- `EvolutionCallModal.vue` renders status, destination, mute, and hang-up
  controls;
- `useEvolutionCall.js` owns the call state and resource lifecycle;
- an Evolution call API client obtains sessions and sends authenticated call
  requests;
- a dedicated audio worklet handles PCM capture and playback.

The destination is initialized from `currentContact.phone_number`, normalized to
digits with DDI and DDD, and remains editable before dialing. WACalls state does
not reuse Chatwoot's Twilio store or `FloatingCallWidget`.

## Browser Call Flow

1. The agent opens an eligible conversation and clicks the phone button.
2. Chatwoot requests microphone permission before initiating the WhatsApp call.
3. Chatwoot obtains a temporary session through its same-origin broker.
4. The client sends `POST /call/start` with the Bearer token and stable
   `X-Call-Client-ID`.
5. It creates a `pcm` data channel and local SDP offer.
6. It sends the offer to `POST /call/:callId/webrtc` and applies the answer.
7. Audio starts after the data channel opens.
8. The modal polls `GET /call/active` for normalized status in this outbound-only
   release.
9. Mute controls the local microphone track and capture worklet.
10. Hang-up sends `DELETE /call/:callId` and releases every browser resource.

If a call lasts beyond the original token lifetime, the API client obtains a new
session before the next HTTP control request. The stable client ID preserves
ownership.

## Error Handling

- Missing broker configuration returns `503` without affecting other APIs.
- Invalid HMAC, stale timestamp, or replayed nonce returns `401`.
- A valid signature with mismatched Chatwoot origin/account/inbox returns `403`.
- Missing, expired, or mismatched call token returns `401` or `403`.
- Ineligible inboxes do not show an active call button.
- Invalid phone numbers are rejected before microphone or network work starts.
- Microphone denial produces an actionable message and does not start a call.
- If call creation succeeds but WebRTC setup fails, the client makes a best-effort
  call termination request.
- Closing or navigating away during an active call triggers best-effort hang-up
  and always stops tracks, worklets, timers, data channels, and peer connections.
- Secrets, Bearer tokens, SDP, and raw audio are excluded from logs.

## Deployment

Evolution GO changes are deployed first with the shared secret configured. The
existing call Manager remains usable during this deployment.

The Chatwoot integration is distributed as a guarded installer script. It uses
`/opt/chatwoot` by default, accepts an explicit source directory and Compose file,
checks expected source markers before changing anything, creates a timestamped
backup, installs backend and frontend files, updates configuration, and rebuilds
the Chatwoot image. A version mismatch aborts before mutation.

The installer never prints the shared secret. It prompts securely or accepts an
already configured environment value. The same generated value must be placed
in the Evolution GO environment before the Chatwoot integration is enabled.

## Testing

Evolution GO tests cover:

- canonical HMAC verification and constant-time rejection;
- timestamp skew and nonce replay;
- Chatwoot origin/account/inbox matching;
- issuance rate limits;
- token hashing, expiry, and client-ID binding;
- Bearer access limited to call routes;
- unchanged `apikey` call authentication.

Chatwoot tests cover:

- authenticated account, conversation, and inbox authorization;
- rejection of non-API or malformed webhook inboxes;
- deterministic signature generation;
- no secret or webhook token in the browser response;
- button eligibility and phone normalization;
- session renewal;
- WebRTC success, failure cleanup, mute, and hang-up behavior.

Verification includes focused Go, Rails, and JavaScript tests followed by the
normal Evolution GO test suite and production builds for both applications.

## Acceptance Criteria

- An authorized Chatwoot agent sees one call button in an eligible Evolution API
  conversation.
- Clicking it opens a native modal with the contact phone number prefilled.
- The agent can start, mute, and end a WhatsApp voice call with working browser
  audio.
- No Evolution instance API key appears in browser storage, responses, source,
  or network requests.
- A stolen temporary token cannot be used for another instance, client ID, or
  non-call endpoint and expires within ten minutes.
- Existing Evolution Manager call tests continue to work with instance API keys.
- Unsupported inboxes and ordinary Chatwoot conversations are unchanged.
