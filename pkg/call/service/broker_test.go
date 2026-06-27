package call_service

import (
	"testing"
	"time"
)

func TestBrokerFiltersOwnedCallsAndUnclaimedOffers(t *testing.T) {
	b := newEventBroker(4)
	sub := b.subscribe("instance", "client-a")
	defer sub.Close()

	b.publish(Event{Type: EventIncoming, InstanceID: "instance", CallID: "one", Status: StatusOffered})
	b.publish(Event{Type: EventStatus, InstanceID: "instance", CallID: "two", ClientID: "client-b", Status: StatusConnected})

	select {
	case got := <-sub.Events():
		if got.CallID != "one" {
			t.Fatalf("unexpected event: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for visible event")
	}
	select {
	case got := <-sub.Events():
		t.Fatalf("received another client's event: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestBrokerDisconnectsSlowSubscriber(t *testing.T) {
	b := newEventBroker(1)
	sub := b.subscribe("instance", "")
	b.publish(Event{Type: EventStatus, InstanceID: "instance", CallID: "one"})
	b.publish(Event{Type: EventStatus, InstanceID: "instance", CallID: "two"})

	if _, ok := <-sub.Events(); !ok {
		return
	}
	if _, ok := <-sub.Events(); ok {
		t.Fatal("slow subscriber channel remained open")
	}
}

func TestBrokerPublishesTerminalEventOnlyOnce(t *testing.T) {
	b := newEventBroker(4)
	sub := b.subscribe("instance", "")
	defer sub.Close()

	event := Event{Type: EventEnded, InstanceID: "instance", CallID: "one", Status: StatusEnded}
	b.publish(event)
	b.publish(event)

	if got := <-sub.Events(); got.Type != EventEnded {
		t.Fatalf("unexpected event: %+v", got)
	}
	select {
	case got := <-sub.Events():
		t.Fatalf("duplicate terminal event: %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
}
