# Chatwoot WACalls Browser Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a loadable Chrome/Edge Manifest V3 extension that maps Chatwoot API inboxes to multiple Evolution profiles and provides outbound WACalls with bidirectional browser audio.

**Architecture:** A trusted service worker stores profile secrets and exposes only typed, allowlisted Evolution operations. A content script discovers the selected Chatwoot conversation, injects an idempotent Shadow DOM call UI, and owns the existing PCM WebRTC flow; a popup manages exact-origin permissions, profiles, site activation, and fallback mappings.

**Tech Stack:** Manifest V3, vanilla JavaScript, Chrome Extensions APIs, WebRTC, AudioWorklet, Node.js test runner, PowerShell packaging.

---

## File Structure

All implementation lives in `extensions/chatwoot-wacalls/`:

- `manifest.json`: MV3 permissions, popup, service worker, packaged resources.
- `shared/core.js`: pure URL, route, webhook, phone, status, and profile mapping helpers.
- `shared/protocol.js`: message names, payload limits, and operation validation.
- `service-worker.js`: trusted storage, permissions, profile verification, and allowlisted Evolution fetches.
- `content.js`: Chatwoot context discovery, button/modal injection, WebRTC, audio, polling, and cleanup.
- `audio-worklet.js`: 16 kHz mono PCM capture and playback processors.
- `popup.html`, `popup.css`, `popup.js`: profile/site/mapping administration.
- `tests/*.test.js`: pure, worker, and content behavior tests.
- `scripts/validate.mjs`: manifest/runtime file validation.
- `scripts/package.ps1`: clean `dist/` and versioned ZIP packaging.
- `package.json`: dependency-free test and validation commands.
- `README.md`: installation, configuration, security boundaries, and troubleshooting.

### Task 1: Scaffold and validate the Manifest V3 package

**Files:**
- Create: `extensions/chatwoot-wacalls/package.json`
- Create: `extensions/chatwoot-wacalls/manifest.json`
- Create: `extensions/chatwoot-wacalls/scripts/validate.mjs`
- Create: `extensions/chatwoot-wacalls/tests/manifest.test.js`

- [ ] **Step 1: Write failing manifest tests**

Assert manifest version 3, classic service worker, `storage/scripting/activeTab` permissions, runtime-discovered optional HTTP/HTTPS hosts, popup, and an audio worklet web-accessible resource. Assert `<all_urls>` is absent from required host permissions.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/manifest.test.js`

Expected: FAIL because `manifest.json` is missing.

- [ ] **Step 3: Add the minimal manifest and package scripts**

Use extension name `Evolution GO WACalls for Chatwoot`, version `0.1.0`, background `service-worker.js`, popup `popup.html`, and optional origins `https://*/*` plus `http://*/*`. Add `npm test`, `npm run validate`, and `npm run package` commands.

- [ ] **Step 4: Implement static validation**

`validate.mjs` loads the manifest, checks every referenced runtime file exists, rejects remote script URLs/eval declarations, and exits nonzero on missing resources.

- [ ] **Step 5: Run GREEN and commit**

Run: `node --test extensions/chatwoot-wacalls/tests/manifest.test.js`

Expected: PASS.

Commit: `feat: scaffold Chatwoot WACalls extension`

### Task 2: Implement pure context and mapping helpers

**Files:**
- Create: `extensions/chatwoot-wacalls/shared/core.js`
- Create: `extensions/chatwoot-wacalls/tests/core.test.js`

- [ ] **Step 1: Write failing helper tests**

Cover normalized HTTPS API origins, supported Chatwoot conversation routes, Evolution webhook parsing without retaining its token, Brazilian/international phone normalization, exact automatic mapping, manual mapping precedence, zero-match, and ambiguous-match results.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/core.test.js`

Expected: FAIL because `WaCallsExtensionCore` is missing.

- [ ] **Step 3: Implement the pure API**

Expose a browser global and CommonJS export containing:

```js
normalizeOrigin(value)
parseChatwootRoute(pathname)
parseEvolutionWebhook(value)
normalizePhone(value)
resolveProfile({ profiles, siteOrigin, accountId, inboxId, webhookUrl })
normalizeCall(value)
isTerminalStatus(status)
```

Reject credentials in URLs, non-HTTP(S) schemes, malformed IDs, and webhook paths other than `/webhooks/chatwoot/:instance/:token`.

- [ ] **Step 4: Run GREEN and commit**

Run: `node --test extensions/chatwoot-wacalls/tests/core.test.js`

Expected: PASS.

Commit: `feat: map Chatwoot inboxes to Evolution profiles`

### Task 3: Build the restricted service-worker boundary

**Files:**
- Create: `extensions/chatwoot-wacalls/shared/protocol.js`
- Create: `extensions/chatwoot-wacalls/service-worker.js`
- Create: `extensions/chatwoot-wacalls/tests/protocol.test.js`
- Create: `extensions/chatwoot-wacalls/tests/service-worker.test.js`

- [ ] **Step 1: Write failing protocol tests**

Assert only `PROFILE_SAVE`, `PROFILE_TEST`, `PROFILE_DELETE`, `SITE_ACTIVATE`, `STATE_GET`, `CALL_START`, `CALL_WEBRTC`, `CALL_ACTIVE`, and `CALL_END` are accepted. Reject caller-supplied URLs, methods, headers, invalid UUID/call IDs, phone numbers shorter than eight digits, and SDP above 256 KiB.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/protocol.test.js extensions/chatwoot-wacalls/tests/service-worker.test.js`

Expected: FAIL because protocol/worker modules are missing.

- [ ] **Step 3: Implement trusted profile storage**

On install/start, call `chrome.storage.local.setAccessLevel({ accessLevel: 'TRUSTED_CONTEXTS' })`. Save API keys only in local storage. Return sanitized profile objects without `apiKey` from every worker response.

- [ ] **Step 4: Implement exact-origin permissions and profile verification**

Request the normalized origin with `chrome.permissions.request`, call `GET /instance/status` with `apikey`, require a successful instance identity, then save verified `instanceId`, `instanceName`, status, and timestamp.

- [ ] **Step 5: Implement allowlisted call operations**

Derive every URL from the stored verified profile. Add `apikey` and stable `X-Call-Client-ID` internally. Implement only `/call/start`, `/call/:id/webrtc`, `/call/active`, and DELETE `/call/:id`, with bounded response parsing and Portuguese error mapping.

- [ ] **Step 6: Run GREEN and commit**

Run: `node --test extensions/chatwoot-wacalls/tests/protocol.test.js extensions/chatwoot-wacalls/tests/service-worker.test.js`

Expected: PASS and tests prove secrets do not cross the message boundary.

Commit: `feat: secure Evolution calls behind extension worker`

### Task 4: Discover Chatwoot context and inject one call button

**Files:**
- Create: `extensions/chatwoot-wacalls/content.js`
- Create: `extensions/chatwoot-wacalls/tests/content.test.js`

- [ ] **Step 1: Write failing context/injection tests**

Use lightweight fake DOM/fetch adapters to assert route changes refresh context, `cw_d_session_info` is held only in memory, conversation/inbox API data is parsed, one button is inserted into `.header-actions-wrap`, reinjection is idempotent, and zero/ambiguous mappings do not add an enabled button.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/content.test.js`

Expected: FAIL because content helpers are missing.

- [ ] **Step 3: Implement context loading**

Parse current account/conversation IDs, read and decode `cw_d_session_info`, make same-origin read-only API calls, discard auth data after each request, parse contact phone and inbox webhook, and request sanitized extension state from the worker.

- [ ] **Step 4: Implement SPA-safe injection**

Use one `data-evolution-wacalls-button` marker, a debounced `MutationObserver`, `popstate`, and patched history event notifications. Insert before existing more-actions controls, use accessible text/title, and remove stale UI when context becomes ineligible.

- [ ] **Step 5: Run GREEN and commit**

Run: `node --test extensions/chatwoot-wacalls/tests/content.test.js`

Expected: PASS.

Commit: `feat: inject WACalls button into Chatwoot`

### Task 5: Add the Shadow DOM modal and PCM WebRTC call flow

**Files:**
- Modify: `extensions/chatwoot-wacalls/content.js`
- Create: `extensions/chatwoot-wacalls/audio-worklet.js`
- Create: `extensions/chatwoot-wacalls/tests/call-session.test.js`
- Create: `extensions/chatwoot-wacalls/tests/audio-worklet.test.js`

- [ ] **Step 1: Write failing call lifecycle tests**

Inject fake media, audio nodes, runtime messaging, and peer connection. Assert microphone setup completes before `CALL_START`, the data channel is named `pcm`, SDP is relayed through `CALL_WEBRTC`, received PCM reaches playback, mute disables track/capture, active status is polled, and every error/end path releases tracks, timers, nodes, AudioContext, channel, and peer.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/call-session.test.js extensions/chatwoot-wacalls/tests/audio-worklet.test.js`

Expected: FAIL because call/media classes are missing.

- [ ] **Step 3: Implement packaged worklets**

Port the tested 16 kHz mono capture/playback framing from the existing Manager asset. Support enable/disable messages, bounded playback buffering, and deterministic processor tests.

- [ ] **Step 4: Implement modal and call session**

Render isolated styles/content in a Shadow DOM root. Implement phone validation, microphone-first start, ICE gathering timeout, 12-second channel timeout, PCM backpressure, duration, status polling, mute, hang-up, modal-close confirmation, unload cleanup, and best-effort DELETE after post-start failure.

- [ ] **Step 5: Run GREEN and commit**

Run: `node --test extensions/chatwoot-wacalls/tests/call-session.test.js extensions/chatwoot-wacalls/tests/audio-worklet.test.js`

Expected: PASS.

Commit: `feat: handle WACalls audio in Chatwoot extension`

### Task 6: Implement profile, site, and mapping administration

**Files:**
- Create: `extensions/chatwoot-wacalls/popup.html`
- Create: `extensions/chatwoot-wacalls/popup.css`
- Create: `extensions/chatwoot-wacalls/popup.js`
- Create: `extensions/chatwoot-wacalls/tests/popup.test.js`

- [ ] **Step 1: Write failing popup state tests**

Assert add/edit/test/delete profile messages, masked API key behavior, current-tab site activation, detected mapping display, manual mapping save/remove, permission errors, and no secret rendering in HTML/text.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/popup.test.js`

Expected: FAIL because popup controller is missing.

- [ ] **Step 3: Implement accessible popup UI**

Create a compact Portuguese UI with labeled fields, status badges, validation, confirmation before profile deletion, and an "Ativar neste Chatwoot" action that operates only from a supported HTTP(S) dashboard tab.

- [ ] **Step 4: Implement manual mapping fallback**

When an eligible inbox is detected without a unique automatic match, list verified profiles and send a typed mapping update containing only Chatwoot origin/account/inbox and profile ID.

- [ ] **Step 5: Run GREEN and commit**

Run: `node --test extensions/chatwoot-wacalls/tests/popup.test.js`

Expected: PASS.

Commit: `feat: configure WACalls profiles from extension popup`

### Task 7: Package and verify Chrome/Edge installation

**Files:**
- Create: `extensions/chatwoot-wacalls/scripts/package.ps1`
- Create: `extensions/chatwoot-wacalls/README.md`
- Modify: `.gitignore` if `dist/` and ZIP artifacts are not already ignored

- [ ] **Step 1: Write validation expectations**

Extend manifest tests to enumerate the exact runtime payload and reject test files, API keys, `.env`, docs drafts, remote URLs, and source maps from `dist/`.

- [ ] **Step 2: Run RED**

Run: `node --test extensions/chatwoot-wacalls/tests/manifest.test.js`

Expected: FAIL because the distribution payload is absent.

- [ ] **Step 3: Implement packaging**

The PowerShell script removes only `extensions/chatwoot-wacalls/dist`, copies the allowlisted runtime files, validates them, and creates `evolution-wacalls-chatwoot-0.1.0.zip` with `Compress-Archive`. Verify resolved paths remain inside the extension directory before removal.

- [ ] **Step 4: Document installation and security boundaries**

Document Chrome `chrome://extensions`, Edge `edge://extensions`, developer mode, Load unpacked, profile setup, site activation, automatic/manual mapping, microphone permission, upgrade, removal, and the device-owner credential limitation.

- [ ] **Step 5: Run complete verification**

Run:

```text
node --test extensions/chatwoot-wacalls/tests/*.test.js
node extensions/chatwoot-wacalls/scripts/validate.mjs
powershell -ExecutionPolicy Bypass -File extensions/chatwoot-wacalls/scripts/package.ps1
```

Expected: all tests pass, validation exits 0, `dist/manifest.json` exists, and the versioned ZIP is produced.

- [ ] **Step 6: Inspect secrets and commit**

Run `rg -n "87\.99\.|evogo\.melck|apikey\s*[:=]\s*['\"]" extensions/chatwoot-wacalls` and confirm no real credential or environment-specific endpoint is packaged.

Commit: `feat: package Chrome and Edge WACalls extension`
