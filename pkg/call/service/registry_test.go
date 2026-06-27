package call_service

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRegistryClaimIsAtomic(t *testing.T) {
	r := newRegistry(time.Minute, 32)
	if err := r.add(newIncomingCall("instance", "call", "peer")); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, clientID := range []string{"client-a", "client-b"} {
		wg.Add(1)
		go func(clientID string) {
			defer wg.Done()
			<-start
			results <- r.claim("instance", "call", clientID)
		}(clientID)
	}
	close(start)
	wg.Wait()
	close(results)

	var claimed, conflicts int
	for err := range results {
		switch {
		case err == nil:
			claimed++
		case errors.Is(err, ErrCallOwned):
			conflicts++
		default:
			t.Fatalf("unexpected claim error: %v", err)
		}
	}
	if claimed != 1 || conflicts != 1 {
		t.Fatalf("claimed=%d conflicts=%d", claimed, conflicts)
	}
}

func TestRegistryLimitsOneActiveCallPerClient(t *testing.T) {
	r := newRegistry(time.Minute, 32)
	if err := r.add(newOutgoingCall("instance", "one", "client", "peer")); err != nil {
		t.Fatal(err)
	}
	if err := r.add(newOutgoingCall("instance", "two", "client", "peer")); !errors.Is(err, ErrClientBusy) {
		t.Fatalf("expected ErrClientBusy, got %v", err)
	}
}

func TestRegistryTerminalTransitionIsIdempotentAndReleasesOwner(t *testing.T) {
	r := newRegistry(time.Minute, 32)
	if err := r.add(newOutgoingCall("instance", "one", "client", "peer")); err != nil {
		t.Fatal(err)
	}
	first, changed, err := r.terminate("instance", "one", StatusEnded, "local")
	if err != nil || !changed || first.Status != StatusEnded {
		t.Fatalf("first terminal transition: snapshot=%+v changed=%v err=%v", first, changed, err)
	}
	second, changed, err := r.terminate("instance", "one", StatusFailed, "duplicate")
	if err != nil || changed || second.Status != StatusEnded || second.Reason != "local" {
		t.Fatalf("duplicate terminal transition: snapshot=%+v changed=%v err=%v", second, changed, err)
	}
	if err := r.add(newOutgoingCall("instance", "two", "client", "peer")); err != nil {
		t.Fatalf("owner was not released: %v", err)
	}
}

func TestRegistryRejectsTerminalToActiveTransition(t *testing.T) {
	r := newRegistry(time.Minute, 32)
	if err := r.add(newOutgoingCall("instance", "one", "client", "peer")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.terminate("instance", "one", StatusRejected, "declined"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.transition("instance", "one", StatusConnected); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestRegistryActiveFiltersByClient(t *testing.T) {
	r := newRegistry(time.Minute, 32)
	if err := r.add(newIncomingCall("instance", "incoming", "peer")); err != nil {
		t.Fatal(err)
	}
	if err := r.add(newOutgoingCall("instance", "owned", "client-a", "peer")); err != nil {
		t.Fatal(err)
	}
	if got := r.active("instance", "client-b"); len(got) != 1 || got[0].CallID != "incoming" {
		t.Fatalf("unexpected filtered calls: %+v", got)
	}
}

func TestRegistryConditionalTerminationRejectsStaleOfferTimeout(t *testing.T) {
	r := newRegistry(time.Minute, 32)
	if err := r.add(newIncomingCall("instance", "call", "peer")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.transition("instance", "call", StatusStarting); err != nil {
		t.Fatal(err)
	}
	call, changed, err := r.terminateIfStatus("instance", "call", StatusOffered, StatusRejected, "timeout")
	if err != nil {
		t.Fatal(err)
	}
	if changed || call.Status != StatusStarting {
		t.Fatalf("stale timeout changed accepted call: changed=%v call=%+v", changed, call)
	}
}
