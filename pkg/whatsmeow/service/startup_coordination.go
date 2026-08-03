package whatsmeow_service

import (
	"sync"

	"go.mau.fi/whatsmeow/store/sqlstore"
)

type storeContainerProvider struct {
	mu        sync.Mutex
	container *sqlstore.Container
	open      func() (*sqlstore.Container, error)
}

func newStoreContainerProvider(open func() (*sqlstore.Container, error)) *storeContainerProvider {
	return &storeContainerProvider{open: open}
}

func (p *storeContainerProvider) Get() (*sqlstore.Container, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.container != nil {
		return p.container, nil
	}

	container, err := p.open()
	if err != nil {
		return nil, err
	}
	p.container = container
	return container, nil
}

type clientStartGate struct {
	mu       sync.Mutex
	starting map[string]struct{}
}

func newClientStartGate() *clientStartGate {
	return &clientStartGate{starting: make(map[string]struct{})}
}

func (g *clientStartGate) Begin(instanceID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.starting[instanceID]; exists {
		return false
	}
	g.starting[instanceID] = struct{}{}
	return true
}

func (g *clientStartGate) End(instanceID string) {
	g.mu.Lock()
	delete(g.starting, instanceID)
	g.mu.Unlock()
}
