package whatsmeow_service

import (
	"testing"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func TestDispatchCallEventForwardsRegisteredHandler(t *testing.T) {
	var got any
	service := &whatsmeowService{}
	service.SetCallEventHandler(func(_ *instance_model.Instance, _ *whatsmeow.Client, event any) {
		got = event
	})
	expected := &events.CallTerminate{}
	service.DispatchCallEvent(&instance_model.Instance{Id: "instance"}, nil, expected)
	if got != expected {
		t.Fatalf("handler received %#v", got)
	}
}

func TestDispatchCallEventWithoutHandlerIsSafe(t *testing.T) {
	service := &whatsmeowService{}
	service.DispatchCallEvent(&instance_model.Instance{Id: "instance"}, nil, &events.CallOffer{})
}
