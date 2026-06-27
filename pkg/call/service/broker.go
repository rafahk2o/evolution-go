package call_service

import "sync"

type Subscription struct {
	broker *eventBroker
	ch     chan Event
	once   sync.Once
}

func (s *Subscription) Events() <-chan Event { return s.ch }

func (s *Subscription) Close() {
	s.once.Do(func() {
		if s.broker != nil {
			s.broker.unsubscribe(s)
		}
	})
}

type eventBroker struct {
	mu             sync.Mutex
	buffer         int
	subscribers    map[*Subscription]subscriptionFilter
	terminalEvents map[registryKey]struct{}
}

type subscriptionFilter struct {
	instanceID string
	clientID   string
}

func newEventBroker(buffer int) *eventBroker {
	if buffer < 1 {
		buffer = 1
	}
	return &eventBroker{
		buffer:         buffer,
		subscribers:    make(map[*Subscription]subscriptionFilter),
		terminalEvents: make(map[registryKey]struct{}),
	}
}

func (b *eventBroker) subscribe(instanceID, clientID string) *Subscription {
	sub := &Subscription{broker: b, ch: make(chan Event, b.buffer)}
	b.mu.Lock()
	b.subscribers[sub] = subscriptionFilter{instanceID: instanceID, clientID: clientID}
	b.mu.Unlock()
	return sub
}

func (b *eventBroker) unsubscribe(sub *Subscription) {
	b.mu.Lock()
	if _, exists := b.subscribers[sub]; exists {
		delete(b.subscribers, sub)
		close(sub.ch)
	}
	b.mu.Unlock()
}

func (b *eventBroker) publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if event.Type == EventEnded {
		key := registryKey{event.InstanceID, event.CallID}
		if _, duplicate := b.terminalEvents[key]; duplicate {
			return
		}
		b.terminalEvents[key] = struct{}{}
	}
	for sub, filter := range b.subscribers {
		if !visibleTo(filter, event) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			delete(b.subscribers, sub)
			close(sub.ch)
		}
	}
}

func visibleTo(filter subscriptionFilter, event Event) bool {
	if filter.instanceID != event.InstanceID {
		return false
	}
	if filter.clientID == "" {
		return true
	}
	if event.ClientID == filter.clientID {
		return true
	}
	return event.ClientID == "" && event.Status == StatusOffered
}
