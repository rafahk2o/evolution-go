# WaCalls Evolution Go Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add authenticated incoming and outgoing WhatsApp voice calls with browser PCM over WebRTC, per-instance ownership, SSE events, and legacy rejection compatibility.

**Architecture:** Import the MIT WaCalls protocol/media engine at commit `edeb31f0427aba896639db503153b777a405eccf` into `internal/wacalls`, preserving licenses and adapting imports to this module. A concurrency-safe call service owns lifecycle, state, WebRTC bridges, retention, and event publication while the existing `whatsmeow.Client` remains the only WhatsApp session. Gin handlers expose the public contract and the existing Whatsmeow event handler forwards native call events into the service without suppressing existing webhooks.

**Tech Stack:** Go 1.25, whatsmeow submodule `0923702`, Pion WebRTC v4, Gin, SSE, Go race detector.

---

### Task 1: Import the WaCalls protocol and media engine

**Files:**
- Create: `internal/wacalls/{call,core,media,signaling,transport,wanode,whatsapp}/**`
- Create: `internal/wacalls/LICENSE`
- Create: `internal/wacalls/UPSTREAM.md`
- Modify: `go.mod`
- Modify: `go.sum`

- [x] **Step 1: Write a failing import test**

```go
package media_test

import (
    "testing"
    "github.com/EvolutionAPI/evolution-go/internal/wacalls/media"
)

func TestPCMInt16LERoundTrip(t *testing.T) {
    got := media.PCMFloat32ToInt16LE([]float32{-1, 0, 1})
    roundTrip := media.PCMInt16LEToFloat32(got)
    if len(roundTrip) != 3 || roundTrip[0] > -0.99 || roundTrip[2] < 0.99 {
        t.Fatalf("unexpected round trip: %v", roundTrip)
    }
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./internal/wacalls/media`
Expected: FAIL because `internal/wacalls/media` does not exist.

- [x] **Step 3: Import and adapt the exact upstream engine**

Copy `internal/voip/{call,core,media,signaling,transport,wanode}` and `internal/wa/socket.go` from WaCalls commit `edeb31f0427aba896639db503153b777a405eccf`. Rewrite imports from `wacalls/internal/voip/...` to `github.com/EvolutionAPI/evolution-go/internal/wacalls/...`, place the socket in package `internal/wacalls/whatsapp`, and preserve both upstream `LICENSE` files. Add Pion WebRTC v4 and the direct cryptographic/network dependencies selected by `go mod tidy`.

- [x] **Step 4: Verify GREEN**

Run: `go test ./internal/wacalls/...`
Expected: PASS for imported foundation, signaling, codec, SRTP, and transport tests.

- [x] **Step 5: Commit**

```powershell
git add internal/wacalls go.mod go.sum
git commit -m "feat: import WaCalls protocol and media engine"
```

### Task 2: Add the concurrency-safe registry and lifecycle model

**Files:**
- Create: `pkg/call/service/types.go`
- Create: `pkg/call/service/errors.go`
- Create: `pkg/call/service/registry.go`
- Create: `pkg/call/service/registry_test.go`

- [x] **Step 1: Write failing registry tests**

```go
func TestRegistryClaimIsAtomic(t *testing.T) {
    r := newRegistry(time.Minute, 32)
    call := newIncomingCall("instance", "call", "peer")
    require.NoError(t, r.add(call))
    require.NoError(t, r.claim("instance", "call", "client-a"))
    require.ErrorIs(t, r.claim("instance", "call", "client-b"), ErrCallOwned)
}

func TestRegistryLimitsOneActiveCallPerClient(t *testing.T) {
    r := newRegistry(time.Minute, 32)
    require.NoError(t, r.add(newOutgoingCall("instance", "one", "client", "peer")))
    require.ErrorIs(t, r.add(newOutgoingCall("instance", "two", "client", "peer")), ErrClientBusy)
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./pkg/call/service -run Registry`
Expected: FAIL because registry symbols are undefined.

- [x] **Step 3: Implement the registry**

Use one mutex for instance/call and instance/client indexes. Define public states `offered`, `starting`, `ringing`, `connected`, `ending`, `ended`, `rejected`, and `failed`; reject transitions from terminal states; remove ownership on terminal transition; retain at most 256 terminal snapshots for 60 seconds; make terminal completion and repeated remote events idempotent.

- [x] **Step 4: Verify GREEN and race safety**

Run: `go test -race ./pkg/call/service -run Registry`
Expected: PASS with no race report.

- [x] **Step 5: Commit**

```powershell
git add pkg/call/service
git commit -m "feat: add call registry and ownership"
```

### Task 3: Add normalized events and bounded SSE subscriptions

**Files:**
- Create: `pkg/call/service/broker.go`
- Create: `pkg/call/service/broker_test.go`

- [x] **Step 1: Write failing broker tests**

```go
func TestBrokerFiltersOwnedCallsAndUnclaimedOffers(t *testing.T) {
    b := newEventBroker(1)
    sub := b.subscribe("instance", "client-a")
    b.publish(Event{Type: "call.incoming", InstanceID: "instance", CallID: "one", Status: StatusOffered})
    b.publish(Event{Type: "call.status", InstanceID: "instance", CallID: "two", ClientID: "client-b"})
    require.Equal(t, "one", (<-sub.Events()).CallID)
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./pkg/call/service -run Broker`
Expected: FAIL because broker symbols are undefined.

- [x] **Step 3: Implement the broker**

Publish `call.incoming`, `call.status`, and exactly one `call.ended` envelope. Give each subscriber a bounded channel; remove and close a subscriber when its buffer fills. Deliver instance-only events, plus unclaimed offers and calls owned by the subscriber when a client filter is present.

- [x] **Step 4: Verify GREEN**

Run: `go test -race ./pkg/call/service -run Broker`
Expected: PASS with the slow subscriber disconnected.

- [x] **Step 5: Commit**

```powershell
git add pkg/call/service/broker.go pkg/call/service/broker_test.go
git commit -m "feat: add normalized call event broker"
```

### Task 4: Add the WebRTC PCM bridge

**Files:**
- Create: `pkg/call/service/webrtc.go`
- Create: `pkg/call/service/webrtc_test.go`

- [x] **Step 1: Write a failing peer negotiation test**

```go
func TestWebRTCBridgeNegotiatesPCMDataChannel(t *testing.T) {
    offerer, offer := localPCMOffer(t)
    bridge, answer, err := newWebRTCBridge(context.Background(), offer.SDP)
    require.NoError(t, err)
    t.Cleanup(func() { bridge.Close() })
    require.NoError(t, offerer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}))
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./pkg/call/service -run WebRTCBridge`
Expected: FAIL because bridge symbols are undefined.

- [x] **Step 3: Implement the bridge**

Use a Pion peer connection and accept only the `pcm` data channel. Convert signed 16-bit little-endian mono 16 kHz frames with the imported media helpers, enforce a 30-second negotiation context, and close data channel and peer connection idempotently.

- [x] **Step 4: Verify GREEN**

Run: `go test -race ./pkg/call/service -run WebRTCBridge`
Expected: PASS and local ICE completes.

- [x] **Step 5: Commit**

```powershell
git add pkg/call/service/webrtc.go pkg/call/service/webrtc_test.go
git commit -m "feat: bridge browser PCM over WebRTC"
```

### Task 5: Orchestrate WhatsApp signaling, media, ownership, and cleanup

**Files:**
- Rewrite: `pkg/call/service/call_service.go`
- Create: `pkg/call/service/call_service_test.go`

- [x] **Step 1: Write failing service behavior tests**

```go
func TestIncomingCallClaimsOnFirstValidWebRTC(t *testing.T) {
    svc := newTestService(t)
    svc.handleIncoming("instance", fakeIncoming("call"))
    _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-a", validOffer(t))
    require.NoError(t, err)
    _, err = svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-b", validOffer(t))
    require.ErrorIs(t, err, ErrCallOwned)
}

func TestAcceptRequiresOwnerAndReadyBridge(t *testing.T) {
    svc := newTestService(t)
    svc.handleIncoming("instance", fakeIncoming("call"))
    require.ErrorIs(t, svc.Accept(context.Background(), "instance", "call", "client-a"), ErrWebRTCNotReady)
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./pkg/call/service -run 'IncomingCall|AcceptRequires'`
Expected: FAIL because the orchestration API is absent.

- [x] **Step 3: Implement service orchestration**

Create one imported `CallManager` per call using the existing `whatsmeow.Client`. Route offers, accepts, transports, rejects, and terminations by call ID. Connect peer PCM callbacks to the bridge and browser PCM callbacks to `FeedCapturedPCM`. Implement start, negotiate, accept, reject, end, active list, disconnect cleanup, offer timeout, terminal retention, normalized publication, and the legacy `RejectCall` method.

- [x] **Step 4: Verify GREEN**

Run: `go test -race ./pkg/call/service`
Expected: PASS with no leaked active calls in cleanup assertions.

- [x] **Step 5: Commit**

```powershell
git add pkg/call/service
git commit -m "feat: orchestrate WhatsApp call lifecycle"
```

### Task 6: Forward native Whatsmeow call events without breaking existing events

**Files:**
- Modify: `pkg/whatsmeow/service/whatsmeow.go`
- Create: `pkg/whatsmeow/service/call_events_test.go`
- Modify: `cmd/evolution-go/main.go`

- [x] **Step 1: Write a failing forwarding test**

```go
func TestDispatchCallEventForwardsRegisteredHandler(t *testing.T) {
    var got any
    svc := &whatsmeowService{}
    svc.SetCallEventHandler(func(_ *instance_model.Instance, _ *whatsmeow.Client, event any) { got = event })
    expected := &events.CallTerminate{}
    svc.DispatchCallEvent(&instance_model.Instance{Id: "instance"}, nil, expected)
    require.Same(t, expected, got)
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./pkg/whatsmeow/service -run DispatchCallEvent`
Expected: FAIL because registration methods are undefined.

- [x] **Step 3: Implement forwarding and wiring**

Store the callback behind an RWMutex, dispatch native events from `MyClient.myEventHandler`, and register the call service in `setupRouter`. Preserve current `CallOffer`, `CallAccept`, and `CallTerminate` webhook/queue behavior. Forward `Disconnected` and `LoggedOut` so calls are cleaned up.

- [x] **Step 4: Verify GREEN**

Run: `go test -race ./pkg/whatsmeow/service ./cmd/evolution-go`
Expected: PASS.

- [x] **Step 5: Commit**

```powershell
git add pkg/whatsmeow/service cmd/evolution-go/main.go
git commit -m "feat: forward WhatsApp call events"
```

### Task 7: Expose authenticated Gin HTTP and SSE contracts

**Files:**
- Rewrite: `pkg/call/handler/call_handler.go`
- Create: `pkg/call/handler/call_handler_test.go`
- Modify: `pkg/routes/routes.go`

- [x] **Step 1: Write failing HTTP contract tests**

```go
func TestStartRequiresCallClientID(t *testing.T) {
    response := requestCallRoute(t, http.MethodPost, "/call/start", `{"number":"5511999999999"}`, nil)
    require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestOwnershipConflictMapsTo409(t *testing.T) {
    response := requestWithServiceError(t, call_service.ErrCallOwned)
    require.Equal(t, http.StatusConflict, response.Code)
}
```

- [x] **Step 2: Verify RED**

Run: `go test ./pkg/call/handler`
Expected: FAIL because the routes and methods are absent.

- [x] **Step 3: Implement handlers and routes**

Expose `POST /call/start`, `POST /call/:callId/webrtc`, `POST /call/:callId/accept`, `POST /call/:callId/reject`, `DELETE /call/:callId`, `GET /call/active`, and `GET /call/events` under the current authentication middleware. Keep `POST /call/reject`, remove its JID middleware mismatch, validate `X-Call-Client-ID` for media/control operations, map domain errors to 400/404/409/422/503/504/500, and add the header to CORS.

- [x] **Step 4: Verify GREEN**

Run: `go test -race ./pkg/call/handler ./pkg/routes`
Expected: PASS.

- [x] **Step 5: Commit**

```powershell
git add pkg/call/handler pkg/routes/routes.go
git commit -m "feat: expose call HTTP and SSE API"
```

### Task 8: Document attribution, API, and verify the complete change

**Files:**
- Modify: `NOTICE`
- Modify: `docs/wiki/guias-api/api-call.md`
- Modify: `docs/swagger.yaml`
- Modify: `docs/swagger.json`
- Modify: `docs/docs.go`

- [ ] **Step 1: Add WaCalls attribution and API documentation**

Document upstream repository, commit, MIT license, PCM format, required ownership header, endpoint bodies/responses, SSE envelope, error statuses, timeouts, and manual two-account validation steps. Regenerate Swagger from handler annotations with the project Swagger command.

- [ ] **Step 2: Run focused verification**

Run: `go test -race ./internal/wacalls/... ./pkg/call/... ./pkg/whatsmeow/service`
Expected: PASS with no race report.

- [ ] **Step 3: Run complete verification**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Build the server**

Run: `go build ./cmd/evolution-go`
Expected: PASS and an `evolution-go` binary.

- [ ] **Step 5: Commit**

```powershell
git add NOTICE docs
git commit -m "docs: document WhatsApp calling API"
```
