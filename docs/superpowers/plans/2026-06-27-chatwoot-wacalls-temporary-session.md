# Chatwoot WACalls Temporary Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a native outbound WACalls button to Chatwoot while authenticating browser call requests with short-lived, server-issued Evolution tokens instead of instance API keys.

**Architecture:** Chatwoot authenticates the agent and signs a server-to-server session request with HMAC. Evolution validates the signed inbox context, stores only a hash of a ten-minute opaque token, and accepts that Bearer token only on `/call/*`; the browser then negotiates the existing PCM WebRTC channel directly with Evolution.

**Tech Stack:** Go, Gin, GORM, Ruby on Rails, HTTParty, Vue 3, Vitest, WebRTC, AudioWorklet, Docker Compose, Bash.

---

## File Structure

Evolution GO owns session issuance and call-route authentication:

- Create `pkg/call/session/types.go` for signed request and temporary-session types.
- Create `pkg/call/session/store.go` for hashed opaque tokens, nonce replay protection, expiry, and issuance limits.
- Create `pkg/call/session/signature.go` for canonical HMAC validation.
- Create `pkg/call/session/service.go` for instance/Chatwoot validation and token issuance.
- Create focused `_test.go` files beside each session file.
- Create `pkg/call/session/handler.go` for `POST /call/session`.
- Modify `pkg/config/env/env.go` and `pkg/config/config.go` for the optional shared secret.
- Modify `pkg/middleware/auth_middleware.go` to add `AuthCall` without widening Bearer access.
- Modify `pkg/routes/routes.go` and `cmd/evolution-go/main.go` to compose the session service and routes.
- Modify `.env.example`, `docker-compose.local.yml`, and call API documentation.

Chatwoot owns agent authorization, UI, and browser media:

- Create `app/services/evolution_calls/session_service.rb` for webhook parsing and signed server-to-server requests.
- Create `app/controllers/api/v1/accounts/evolution_calls_controller.rb` for account/conversation/inbox authorization.
- Modify `config/routes.rb` to expose the broker endpoint.
- Create `app/javascript/dashboard/api/evolutionCalls.js` for the same-origin session request and direct Evolution call requests.
- Create `app/javascript/dashboard/composables/useEvolutionCall.js` for token renewal and WebRTC lifecycle.
- Create `app/javascript/dashboard/components/widgets/conversation/EvolutionCallButton.vue` and `EvolutionCallModal.vue`.
- Modify `ConversationHeader.vue` to mount one button before `MoreActions`.
- Create `public/evolution-wacalls-audio-worklet.js` for 16 kHz mono PCM capture/playback.
- Add Rails and Vitest tests beside existing project test groups.

Distribution lives in Evolution GO:

- Create `integrations/chatwoot/overlay/` from the tested Chatwoot additions.
- Create `integrations/chatwoot/chatwoot-wacalls.patch` from the tested modifications.
- Create `integrations/chatwoot/install.sh` to validate, back up, install, configure, and rebuild Chatwoot.

### Task 1: Load optional shared-session configuration

**Files:**
- Modify: `pkg/config/env/env.go`
- Modify: `pkg/config/config.go`
- Modify: `.env.example`
- Modify: `docker-compose.local.yml`
- Test: `pkg/config/config_test.go`

- [ ] **Step 1: Write a failing config test**

Add a test that sets `CHATWOOT_CALL_SESSION_SECRET=session-secret`, calls the existing config loader with the required test environment, and asserts:

```go
require.Equal(t, "session-secret", cfg.ChatwootCallSessionSecret)
```

- [ ] **Step 2: Run the focused test and confirm the field is missing**

Run: `go test ./pkg/config -run TestLoadChatwootCallSessionSecret -count=1`

Expected: compile failure because `ChatwootCallSessionSecret` does not exist.

- [ ] **Step 3: Add the optional field and environment constant**

Add:

```go
const CHATWOOT_CALL_SESSION_SECRET = "CHATWOOT_CALL_SESSION_SECRET"
```

and:

```go
ChatwootCallSessionSecret string
```

Load it with `os.Getenv` without `panicIfEmpty`, so normal operation remains available when the broker is disabled.

- [ ] **Step 4: Expose the variable to local Compose and document generation**

Add to `.env.example`:

```env
CHATWOOT_CALL_SESSION_SECRET=
```

Add to the Evolution service environment:

```yaml
CHATWOOT_CALL_SESSION_SECRET: ${CHATWOOT_CALL_SESSION_SECRET:-}
```

- [ ] **Step 5: Run the test and commit**

Run: `go test ./pkg/config -run TestLoadChatwootCallSessionSecret -count=1`

Expected: PASS.

Commit: `feat: load Chatwoot call session secret`

### Task 2: Build the temporary session store

**Files:**
- Create: `pkg/call/session/types.go`
- Create: `pkg/call/session/store.go`
- Create: `pkg/call/session/store_test.go`

- [ ] **Step 1: Write failing tests for hashing, expiry, and client binding**

Cover a deterministic clock and random reader. Required assertions:

```go
issued, err := store.Issue(SessionClaims{InstanceID: "i1", ClientID: "c1", AgentID: "a1"})
require.NoError(t, err)
require.NotEmpty(t, issued.AccessToken)
require.NotContains(t, fmt.Sprintf("%v", store.sessions), issued.AccessToken)

claims, ok := store.Authenticate(issued.AccessToken, "c1")
require.True(t, ok)
require.Equal(t, "i1", claims.InstanceID)

_, ok = store.Authenticate(issued.AccessToken, "other-client")
require.False(t, ok)
```

Advance the fake clock by more than ten minutes and assert authentication fails.

- [ ] **Step 2: Run and confirm the package does not exist**

Run: `go test ./pkg/call/session -run 'TestStore' -count=1`

Expected: FAIL because the package implementation is missing.

- [ ] **Step 3: Implement focused session types and store**

Define:

```go
type SessionClaims struct {
    InstanceID, AccountID, InboxID, ConversationID, AgentID, ClientID string
}

type IssuedSession struct {
    AccessToken string    `json:"accessToken"`
    TokenType   string    `json:"tokenType"`
    ExpiresAt   time.Time `json:"expiresAt"`
    InstanceID  string    `json:"instanceId"`
    InstanceName string   `json:"instanceName"`
}
```

Generate 32 random bytes with `crypto/rand`, encode using base64 raw URL encoding, and index the in-memory map by `sha256.Sum256([]byte(token))`. Guard maps with `sync.Mutex`, use injected `now` and `io.Reader`, and remove expired entries during issue/authenticate operations.

- [ ] **Step 4: Add nonce replay and bounded issuance tests**

Assert `UseNonce(nonce, timestamp)` succeeds once, fails on reuse, and succeeds again only after replay retention. Assert more than the configured per-agent/window limit returns `ErrRateLimited`.

- [ ] **Step 5: Implement nonce and issuance windows, then run tests**

Run: `go test ./pkg/call/session -run 'TestStore|TestNonce|TestRate' -count=1`

Expected: PASS with `-race` also clean:

`go test -race ./pkg/call/session -count=1`

- [ ] **Step 6: Commit**

Commit: `feat: add temporary call session store`

### Task 3: Verify HMAC and configured Chatwoot context

**Files:**
- Create: `pkg/call/session/signature.go`
- Create: `pkg/call/session/signature_test.go`
- Create: `pkg/call/session/service.go`
- Create: `pkg/call/session/service_test.go`

- [ ] **Step 1: Write signature tests**

Use the exact canonical input:

```go
canonical := timestamp + "\n" + nonce + "\n" + string(rawBody)
mac := hmac.New(sha256.New, []byte(secret))
_, _ = mac.Write([]byte(canonical))
signature := hex.EncodeToString(mac.Sum(nil))
```

Assert a valid signature passes and altered body, nonce, timestamp, and signature fail. Assert timestamps outside `now +/- 60s` fail and the same nonce cannot pass twice.

- [ ] **Step 2: Run and observe missing verifier failures**

Run: `go test ./pkg/call/session -run 'TestSignature' -count=1`

Expected: FAIL because `Verifier` is undefined.

- [ ] **Step 3: Implement constant-time signature verification**

Decode lowercase hex and use `hmac.Equal`. Return typed errors for disabled configuration, stale timestamp, replay, and invalid signature. Mark a nonce used only after timestamp and signature validation succeed.

- [ ] **Step 4: Write service tests with a fake instance repository**

The valid fixture must contain:

```go
&instance_model.Instance{
    Id: "i1", Name: "support", ChatwootEnabled: true,
    ChatwootURL: "https://chat.example/",
    ChatwootAccountID: "1", ChatwootInboxID: "12",
}
```

Assert normalized URL, account, and inbox match. Assert disabled Chatwoot and every mismatch return `ErrContextForbidden` without issuing a token.

- [ ] **Step 5: Implement the issuer service**

Parse `SessionRequest`, resolve with `GetInstanceByName`, normalize origins with `net/url`, compare stored configuration, apply the issuance limit, and return `store.Issue` claims bound to the signed agent/client/conversation context.

- [ ] **Step 6: Run tests and commit**

Run: `go test -race ./pkg/call/session -count=1`

Expected: PASS.

Commit: `feat: validate signed Chatwoot call sessions`

### Task 4: Expose session issuance and scoped Bearer authentication

**Files:**
- Create: `pkg/call/session/handler.go`
- Create: `pkg/call/session/handler_test.go`
- Modify: `pkg/middleware/auth_middleware.go`
- Create: `pkg/middleware/call_auth_test.go`
- Modify: `pkg/routes/routes.go`
- Modify: `cmd/evolution-go/main.go`

- [ ] **Step 1: Write handler tests**

Create Gin contexts for `POST /call/session`. Assert missing configuration maps to `503`, invalid signature to `401`, context mismatch to `403`, rate limit to `429`, malformed JSON to `400`, and valid issuance to `201` with no instance API key fields.

- [ ] **Step 2: Implement the issuance handler**

Read at most 32 KiB, preserve raw JSON bytes for HMAC verification, bind `SessionRequest`, read the three signature headers, call the issuer, and map typed errors. Never log request headers/body.

- [ ] **Step 3: Write call middleware tests**

Required cases:

```go
// Existing behavior
req.Header.Set("apikey", instance.Token)
// Temporary behavior
req.Header.Set("Authorization", "Bearer "+issued.AccessToken)
req.Header.Set("X-Call-Client-ID", claims.ClientID)
```

Assert both populate the same `instance` context. Assert Bearer with another client ID fails, and assert a non-call route still uses normal `Auth` and rejects Bearer.

- [ ] **Step 4: Implement `AuthCall`**

Extend the middleware interface with `AuthCall`. If a Bearer token is present, authenticate it through the session store and load its instance by ID; otherwise delegate to the existing instance-token behavior. Do not change `Auth`, `AuthAdmin`, or `AuthMaster` semantics.

- [ ] **Step 5: Wire routes and dependencies**

Construct one session store/verifier/service in `cmd/evolution-go/main.go`. Register `POST /call/session` before the authenticated call group. Change only the `/call` group to `AuthCall`. Pass the session handler into `NewRouter` as a dedicated dependency.

- [ ] **Step 6: Run focused route tests and commit**

Run:

```text
go test ./pkg/call/session ./pkg/middleware ./pkg/call/handler ./pkg/routes -count=1
```

Expected: PASS.

Commit: `feat: authenticate calls with temporary sessions`

### Task 5: Document and verify the Evolution API

**Files:**
- Modify: `docs/wiki/guias-api/api-call.md`
- Modify generated Swagger annotations/files through the repository's existing command
- Test: all Go packages

- [ ] **Step 1: Document the broker contract and security boundaries**

Add request headers, canonical signature text, ten-minute expiry, Bearer examples, refresh behavior, and the guarantee that temporary tokens work only on `/call/*`.

- [ ] **Step 2: Regenerate API documentation**

Run the repository's Swagger generation command identified in the existing Makefile/scripts. Confirm `/call/session` appears and `Authorization` is documented on call routes without removing `apikey` compatibility.

- [ ] **Step 3: Run Go verification**

Run:

```text
gofmt -w pkg/call/session pkg/middleware pkg/routes cmd/evolution-go pkg/config
go vet ./pkg/call/... ./pkg/middleware/... ./pkg/routes/... ./pkg/config/...
go test ./... -count=1
```

Expected: all commands exit 0.

- [ ] **Step 4: Commit**

Commit: `docs: document temporary Chatwoot call sessions`

### Task 6: Implement the authenticated Chatwoot session broker

**Files (Chatwoot repository):**
- Create: `app/services/evolution_calls/session_service.rb`
- Create: `app/controllers/api/v1/accounts/evolution_calls_controller.rb`
- Modify: `config/routes.rb`
- Create: `spec/services/evolution_calls/session_service_spec.rb`
- Create: `spec/controllers/api/v1/accounts/evolution_calls_controller_spec.rb`

- [ ] **Step 1: Write service tests for URL parsing and signature generation**

Build a `Channel::Api` webhook URL ending in `/webhooks/chatwoot/support/token`. Freeze time, stub `SecureRandom.hex`, and verify HTTParty receives the exact JSON plus:

```ruby
canonical = "#{timestamp}\n#{nonce}\n#{raw_body}"
expected = OpenSSL::HMAC.hexdigest('SHA256', secret, canonical)
```

Assert malformed/non-HTTPS webhook URLs are rejected and the webhook token never appears in the response or outbound body.

- [ ] **Step 2: Implement `EvolutionCalls::SessionService`**

Accept `account:, inbox:, conversation:, agent:, client_id:`. Read `ENV['CHATWOOT_CALL_SESSION_SECRET']`, parse the channel webhook with `URI`, create deterministic JSON, sign the raw bytes, call `#{origin}/call/session` with five-second open/read timeouts, and return only the parsed safe fields plus `apiUrl: origin`.

- [ ] **Step 3: Write controller authorization tests**

Assert an authenticated agent can request a session only when the conversation, inbox, and account match. Assert another account, a conversation in another inbox, a non-API inbox, and missing phone/session configuration fail without making an Evolution request.

- [ ] **Step 4: Implement controller and route**

Add inside account routes:

```ruby
resource :evolution_calls, only: [] do
  post :session
end
```

The controller loads records from `Current.account`, authorizes the conversation with the existing policy, derives `Current.user.id`, and delegates to the service. Map upstream timeout/unavailability to `502/503` without returning secrets.

- [ ] **Step 5: Run Rails tests and commit in Chatwoot**

Run:

```text
bundle exec rspec spec/services/evolution_calls/session_service_spec.rb spec/controllers/api/v1/accounts/evolution_calls_controller_spec.rb
bundle exec rubocop app/services/evolution_calls/session_service.rb app/controllers/api/v1/accounts/evolution_calls_controller.rb
```

Expected: PASS.

Commit: `feat: broker temporary Evolution call sessions`

### Task 7: Add the native Chatwoot call button and WebRTC client

**Files (Chatwoot repository):**
- Create: `app/javascript/dashboard/api/evolutionCalls.js`
- Create: `app/javascript/dashboard/composables/useEvolutionCall.js`
- Create: `app/javascript/dashboard/components/widgets/conversation/EvolutionCallButton.vue`
- Create: `app/javascript/dashboard/components/widgets/conversation/EvolutionCallModal.vue`
- Modify: `app/javascript/dashboard/components/widgets/conversation/ConversationHeader.vue`
- Create: `public/evolution-wacalls-audio-worklet.js`
- Create: `app/javascript/dashboard/composables/spec/useEvolutionCall.spec.js`
- Create: `app/javascript/dashboard/components/widgets/conversation/spec/EvolutionCallButton.spec.js`

- [ ] **Step 1: Write button eligibility tests**

Mount with API and non-API inboxes. Assert exactly one button appears only when `channel_type === 'Channel::Api'`, webhook pathname matches `/webhooks/chatwoot/:instance/:token`, and contact phone is present. Assert the button emits/open modal with normalized digits.

- [ ] **Step 2: Implement the button and header integration**

Import `EvolutionCallButton` in `ConversationHeader.vue` and render it immediately before `MoreActions`, passing `chat`, `inbox`, and `currentContact`. Use the existing icon system and accessible label `Ligar via WhatsApp`.

- [ ] **Step 3: Write composable tests for session refresh and cleanup**

Mock `getUserMedia`, `AudioContext`, `AudioWorkletNode`, and `RTCPeerConnection`. Assert microphone setup occurs before `/call/start`, session refresh occurs when expiry is under 30 seconds, negotiation uses a `pcm` data channel, and every failure/close stops tracks and closes nodes/channel/peer.

- [ ] **Step 4: Implement the API client**

The same-origin broker request uses the existing Chatwoot axios client. Direct Evolution requests set:

```js
headers: {
  Authorization: `Bearer ${session.accessToken}`,
  'X-Call-Client-ID': clientId,
  'Content-Type': 'application/json',
}
```

Never persist the token in localStorage; hold it only in composable memory.

- [ ] **Step 5: Implement WebRTC/audio lifecycle and modal**

Adapt the tested Manager PCM flow: request mono microphone audio, load `/evolution-wacalls-audio-worklet.js`, create capture/playback worklets, create `RTCPeerConnection`, create `pcm`, post SDP, apply answer, and wait up to 12 seconds for channel open. Implement start, mute, active polling, hang-up, unload cleanup, and actionable Portuguese errors.

- [ ] **Step 6: Run JavaScript tests, lint, and commit**

Run:

```text
pnpm test app/javascript/dashboard/composables/spec/useEvolutionCall.spec.js app/javascript/dashboard/components/widgets/conversation/spec/EvolutionCallButton.spec.js
pnpm eslint app/javascript/dashboard/api/evolutionCalls.js app/javascript/dashboard/composables/useEvolutionCall.js app/javascript/dashboard/components/widgets/conversation/EvolutionCallButton.vue app/javascript/dashboard/components/widgets/conversation/EvolutionCallModal.vue app/javascript/dashboard/components/widgets/conversation/ConversationHeader.vue
```

Expected: PASS.

Commit: `feat: add Evolution call button to conversations`

### Task 8: Package the guarded Chatwoot installer

**Files (Evolution GO repository):**
- Create: `integrations/chatwoot/overlay/**`
- Create: `integrations/chatwoot/chatwoot-wacalls.patch`
- Create: `integrations/chatwoot/install.sh`
- Create: `integrations/chatwoot/install_test.sh`
- Create: `integrations/chatwoot/README.md`

- [ ] **Step 1: Export tested Chatwoot changes**

Copy every new tested file into `overlay/` preserving its Chatwoot-relative path. Generate a patch containing only modifications to existing files (`config/routes.rb` and `ConversationHeader.vue`). Ensure the patch contains no `.env`, token, secret, generated bundle, or unrelated local change.

- [ ] **Step 2: Write a failing installer smoke test**

Create a minimal temporary Chatwoot fixture with the expected route/header markers. Run `install.sh --chatwoot-dir <fixture> --no-build` and assert files are copied, patch is applied once, a timestamped backup exists, and a second invocation exits successfully without duplicating imports/routes/components.

- [ ] **Step 3: Implement guarded installation**

The Bash script must use `set -Eeuo pipefail`, default `CHATWOOT_DIR=/opt/chatwoot`, require root or writable files, verify `git apply --check`, make a backup before mutation, copy the overlay, apply the patch, securely prompt/generate a 32-byte secret when absent, update `.env` without printing the value, and support `--no-build`.

For deployment, detect the Compose file or accept `--compose-file`. Build and recreate Rails and worker services from the same image. Abort before mutation if source markers or Compose services are unknown.

- [ ] **Step 4: Test idempotence and rollback artifacts**

Run: `bash integrations/chatwoot/install_test.sh`

Expected: PASS and no fixture residue outside the temporary directory.

- [ ] **Step 5: Document the two-server sequence and commit**

Document Evolution deployment first, copying the same secret to Chatwoot without exposing it in shell history, installer invocation, health checks, browser cache refresh, and rollback from the generated backup.

Commit: `feat: package Chatwoot WACalls installer`

### Task 9: Cross-project verification

**Files:**
- Verify all changed files in both repositories

- [ ] **Step 1: Verify clean secret handling**

Search tracked diffs for actual environment values, `apikey`, Bearer tokens, webhook tokens, private server addresses, and generated credentials. Confirm only variable names and synthetic test values exist.

- [ ] **Step 2: Run Evolution verification**

Run:

```text
go test -race ./pkg/call/session ./pkg/middleware ./pkg/call/handler -count=1
go test ./... -count=1
docker compose -f docker-compose.local.yml config
```

Expected: all exit 0.

- [ ] **Step 3: Run Chatwoot verification**

Run focused RSpec/Vitest commands from Tasks 6 and 7, then:

```text
bundle exec rails routes | rg evolution_calls
pnpm build
```

Expected: route present and production build exits 0.

- [ ] **Step 4: Review deployment payload**

Apply the installer to a clean temporary copy at the same Chatwoot revision as the server. Compare the resulting tracked diff with the tested local Chatwoot diff; they must be identical except for `.env` and backup metadata.

- [ ] **Step 5: Commit final verification fixes only if required**

Use a narrowly scoped commit message describing the verified correction. Do not squash unrelated user commits.
