package whatsmeow_service

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

func TestStoreContainerProviderReusesSuccessfulContainer(t *testing.T) {
	want := &sqlstore.Container{}
	var opens atomic.Int32
	provider := newStoreContainerProvider(func() (*sqlstore.Container, error) {
		opens.Add(1)
		return want, nil
	})

	const callers = 20
	results := make(chan *sqlstore.Container, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := provider.Get()
			if err != nil {
				t.Errorf("get shared container: %v", err)
				return
			}
			results <- got
		}()
	}
	wg.Wait()
	close(results)

	for got := range results {
		if got != want {
			t.Fatalf("provider returned container %p, want %p", got, want)
		}
	}
	if got := opens.Load(); got != 1 {
		t.Fatalf("opened store %d times, want 1", got)
	}
}

func TestStoreContainerProviderRetriesAfterTransientFailure(t *testing.T) {
	want := &sqlstore.Container{}
	var opens atomic.Int32
	provider := newStoreContainerProvider(func() (*sqlstore.Container, error) {
		if opens.Add(1) == 1 {
			return nil, errors.New("too many clients already")
		}
		return want, nil
	})

	if _, err := provider.Get(); err == nil {
		t.Fatal("first open succeeded, want transient failure")
	}
	got, err := provider.Get()
	if err != nil {
		t.Fatalf("retry open: %v", err)
	}
	if got != want {
		t.Fatalf("retry returned container %p, want %p", got, want)
	}
	if _, err = provider.Get(); err != nil {
		t.Fatalf("reuse after retry: %v", err)
	}
	if count := opens.Load(); count != 2 {
		t.Fatalf("opened store %d times, want 2", count)
	}
}

func TestClientStartGateAllowsOneConcurrentStartPerInstance(t *testing.T) {
	gate := newClientStartGate()
	if !gate.Begin("instance-a") {
		t.Fatal("first start was rejected")
	}
	if gate.Begin("instance-a") {
		t.Fatal("concurrent start was accepted")
	}
	if !gate.Begin("instance-b") {
		t.Fatal("start for another instance was rejected")
	}

	gate.End("instance-a")
	if !gate.Begin("instance-a") {
		t.Fatal("retry after completed start was rejected")
	}
}
