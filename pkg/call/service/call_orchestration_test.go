package call_service

import (
	"context"
	"errors"
	"sync"
	"testing"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type fakeCallEngine struct {
	mu        sync.Mutex
	callbacks engineCallbacks
	accepted  int
	rejected  int
	ended     int
}

func (f *fakeCallEngine) SetCallbacks(callbacks engineCallbacks)         { f.callbacks = callbacks }
func (f *fakeCallEngine) Start(context.Context, string, types.JID) error { return nil }
func (f *fakeCallEngine) Accept(context.Context, string) error {
	f.mu.Lock()
	f.accepted++
	f.mu.Unlock()
	return nil
}
func (f *fakeCallEngine) Reject(context.Context, string) error {
	f.mu.Lock()
	f.rejected++
	f.mu.Unlock()
	return nil
}
func (f *fakeCallEngine) End(context.Context) error {
	f.mu.Lock()
	f.ended++
	f.mu.Unlock()
	return nil
}
func (f *fakeCallEngine) FeedPCM([]float32)                {}
func (f *fakeCallEngine) HandleEvent(context.Context, any) {}

type fakeMediaBridge struct {
	ready      bool
	closed     bool
	onPCM      func([]float32)
	onTerminal func()
}

func (f *fakeMediaBridge) SetBrowserPCMHandler(handler func([]float32)) { f.onPCM = handler }
func (f *fakeMediaBridge) SetTerminalHandler(handler func())            { f.onTerminal = handler }
func (f *fakeMediaBridge) Ready() bool                                  { return f.ready }
func (f *fakeMediaBridge) WritePCM([]float32) error                     { return nil }
func (f *fakeMediaBridge) Close() error                                 { f.closed = true; return nil }

func incomingEvent(callID string) *events.CallOffer {
	creator := types.NewJID("5511999999999", types.DefaultUserServer)
	return &events.CallOffer{
		BasicCallMeta: types.BasicCallMeta{From: creator, CallCreator: creator, CallID: callID},
		Data:          &waBinary.Node{Tag: "offer", Attrs: waBinary.Attrs{"call-id": callID, "call-creator": creator}},
	}
}

func newOrchestrationTestService(bridgeReady bool) (*callService, *fakeCallEngine, *fakeMediaBridge) {
	engine := &fakeCallEngine{}
	bridge := &fakeMediaBridge{ready: bridgeReady}
	svc := newCallService(callServiceOptions{
		engineFactory: func(*whatsmeow.Client) callEngine { return engine },
		bridgeFactory: func(context.Context, string) (mediaBridge, string, error) {
			return bridge, "answer", nil
		},
	})
	return svc, engine, bridge
}

func TestIncomingCallClaimsOnFirstValidWebRTC(t *testing.T) {
	svc, _, _ := newOrchestrationTestService(true)
	instance := &instance_model.Instance{Id: "instance"}
	svc.HandleWhatsAppEvent(instance, nil, incomingEvent("call"))

	if _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-a", "offer"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-b", "offer"); !errors.Is(err, ErrCallOwned) {
		t.Fatalf("expected ErrCallOwned, got %v", err)
	}
}

func TestAcceptRequiresOwnerAndReadyBridge(t *testing.T) {
	svc, engine, _ := newOrchestrationTestService(false)
	instance := &instance_model.Instance{Id: "instance"}
	svc.HandleWhatsAppEvent(instance, nil, incomingEvent("call"))
	if _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-a", "offer"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Accept(context.Background(), "instance", "call", "client-a"); !errors.Is(err, ErrWebRTCNotReady) {
		t.Fatalf("expected ErrWebRTCNotReady, got %v", err)
	}
	if engine.accepted != 0 {
		t.Fatal("engine accepted call before WebRTC was ready")
	}
}

func TestAcceptRequiresSameOwner(t *testing.T) {
	svc, _, _ := newOrchestrationTestService(true)
	instance := &instance_model.Instance{Id: "instance"}
	svc.HandleWhatsAppEvent(instance, nil, incomingEvent("call"))
	if _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-a", "offer"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Accept(context.Background(), "instance", "call", "client-b"); !errors.Is(err, ErrCallOwned) {
		t.Fatalf("expected ErrCallOwned, got %v", err)
	}
}

func TestEndIsIdempotentAndClosesBridge(t *testing.T) {
	svc, engine, bridge := newOrchestrationTestService(true)
	instance := &instance_model.Instance{Id: "instance"}
	svc.HandleWhatsAppEvent(instance, nil, incomingEvent("call"))
	if _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-a", "offer"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Accept(context.Background(), "instance", "call", "client-a"); err != nil {
		t.Fatal(err)
	}
	first, err := svc.End(context.Background(), "instance", "call", "client-a")
	if err != nil || first.Status != StatusEnded {
		t.Fatalf("first end: %+v, %v", first, err)
	}
	second, err := svc.End(context.Background(), "instance", "call", "client-a")
	if err != nil || second.Status != StatusEnded {
		t.Fatalf("second end: %+v, %v", second, err)
	}
	if !bridge.closed || engine.ended != 1 {
		t.Fatalf("closed=%v ended=%d", bridge.closed, engine.ended)
	}
}

func TestDisconnectedCleansEveryInstanceCall(t *testing.T) {
	svc, _, bridge := newOrchestrationTestService(true)
	instance := &instance_model.Instance{Id: "instance"}
	svc.HandleWhatsAppEvent(instance, nil, incomingEvent("call"))
	if _, err := svc.NegotiateWebRTC(context.Background(), "instance", "call", "client-a", "offer"); err != nil {
		t.Fatal(err)
	}
	svc.HandleWhatsAppEvent(instance, nil, &events.Disconnected{})
	if active := svc.Active("instance", ""); len(active) != 0 {
		t.Fatalf("calls remained active: %+v", active)
	}
	if !bridge.closed {
		t.Fatal("bridge was not closed")
	}
}
