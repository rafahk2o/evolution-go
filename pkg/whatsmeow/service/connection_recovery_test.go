package whatsmeow_service

import (
	"sync"
	"testing"
	"time"

	"github.com/EvolutionAPI/evolution-go/pkg/config"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	instance_repository "github.com/EvolutionAPI/evolution-go/pkg/instance/repository"
	logger_wrapper "github.com/EvolutionAPI/evolution-go/pkg/logger"
	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func TestConfigureConnectionRecoveryKeepsWhatsmeowAutoReconnectEnabled(t *testing.T) {
	client := &whatsmeow.Client{EnableAutoReconnect: false}

	configureConnectionRecovery(client)

	if !client.EnableAutoReconnect {
		t.Fatal("native WhatsApp reconnect is disabled")
	}
}

type connectionRecoveryServiceSpy struct {
	WhatsmeowService
	reconnectOnce sync.Once
	reconnected   chan struct{}
}

func (s *connectionRecoveryServiceSpy) DispatchCallEvent(*instance_model.Instance, *whatsmeow.Client, any) {
}

func (s *connectionRecoveryServiceSpy) ReconnectClient(string) error {
	s.reconnectOnce.Do(func() { close(s.reconnected) })
	return nil
}

func (s *connectionRecoveryServiceSpy) CallWebhook(*instance_model.Instance, string, []byte) {
}

type connectionRecoveryRepository struct {
	instance_repository.InstanceRepository
}

func (connectionRecoveryRepository) UpdateConnected(string, bool, string) error {
	return nil
}

func TestDisconnectedEventLeavesRecoveryToWhatsmeow(t *testing.T) {
	logConfig := &config.Config{LogDirectory: t.TempDir()}
	service := &connectionRecoveryServiceSpy{reconnected: make(chan struct{})}
	instance := &instance_model.Instance{Id: "instance-a", Token: "token-a"}
	myClient := &MyClient{
		service:            service,
		Instance:           instance,
		userID:             instance.Id,
		token:              instance.Token,
		instanceRepository: connectionRecoveryRepository{},
		userInfoCache:      cache.New(cache.NoExpiration, cache.NoExpiration),
		config:             logConfig,
		loggerWrapper:      logger_wrapper.NewLoggerManager(logConfig),
	}

	myClient.myEventHandler(&events.Disconnected{})

	select {
	case <-service.reconnected:
		t.Fatal("service-level restart raced with whatsmeow automatic reconnect")
	case <-time.After(100 * time.Millisecond):
	}
}
