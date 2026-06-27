package call_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	wacall "github.com/EvolutionAPI/evolution-go/internal/wacalls/call"
	wacore "github.com/EvolutionAPI/evolution-go/internal/wacalls/core"
	"github.com/EvolutionAPI/evolution-go/internal/wacalls/signaling"
	waadapter "github.com/EvolutionAPI/evolution-go/internal/wacalls/whatsapp"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/EvolutionAPI/evolution-go/pkg/logger"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	negotiationTimeout = 30 * time.Second
	incomingOfferTTL   = 60 * time.Second
	terminalRetention  = 60 * time.Second
	maxTerminalCalls   = 256
	eventBufferSize    = 32
)

type CallService interface {
	Start(ctx context.Context, instance *instance_model.Instance, clientID, number string) (Call, error)
	NegotiateWebRTC(ctx context.Context, instanceID, callID, clientID, offer string) (string, error)
	Accept(ctx context.Context, instanceID, callID, clientID string) error
	Reject(ctx context.Context, instanceID, callID, clientID string) (Call, error)
	End(ctx context.Context, instanceID, callID, clientID string) (Call, error)
	Active(instanceID, clientID string) []Call
	Subscribe(instanceID, clientID string) *Subscription
	RejectCall(data *RejectCallStruct, instance *instance_model.Instance) error
	HandleWhatsAppEvent(instance *instance_model.Instance, client *whatsmeow.Client, event any)
	CloseInstance(instanceID string)
}

type RejectCallStruct struct {
	CallCreator types.JID `json:"callCreator" binding:"required"`
	CallID      string    `json:"callId" binding:"required"`
}

type engineCallbacks struct {
	State   func(*wacall.CallInfo)
	Ended   func(*wacall.CallInfo)
	PeerPCM func([]float32)
}

type callEngine interface {
	SetCallbacks(engineCallbacks)
	Start(context.Context, string, types.JID) error
	Accept(context.Context, string) error
	Reject(context.Context, string) error
	End(context.Context) error
	FeedPCM([]float32)
	HandleEvent(context.Context, any)
}

type mediaBridge interface {
	SetBrowserPCMHandler(func([]float32))
	SetTerminalHandler(func())
	Ready() bool
	WritePCM([]float32) error
	Close() error
}

type callServiceOptions struct {
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
	loggerWrapper    *logger_wrapper.LoggerManager
	engineFactory    func(*whatsmeow.Client) callEngine
	bridgeFactory    func(context.Context, string) (mediaBridge, string, error)
	publishHook      func(*instance_model.Instance, Event)
	offerTimeout     time.Duration
}

type callService struct {
	registry         *registry
	broker           *eventBroker
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
	loggerWrapper    *logger_wrapper.LoggerManager
	engineFactory    func(*whatsmeow.Client) callEngine
	bridgeFactory    func(context.Context, string) (mediaBridge, string, error)
	publishHook      func(*instance_model.Instance, Event)
	offerTimeout     time.Duration

	mu        sync.RWMutex
	runtimes  map[registryKey]*callRuntime
	instances map[string]*instance_model.Instance
}

type callRuntime struct {
	engine callEngine

	mu         sync.RWMutex
	bridge     mediaBridge
	offerTimer *time.Timer
	closeOnce  sync.Once
}

func newCallService(options callServiceOptions) *callService {
	if options.engineFactory == nil {
		options.engineFactory = func(client *whatsmeow.Client) callEngine {
			return newWhatsAppCallEngine(client)
		}
	}
	if options.bridgeFactory == nil {
		options.bridgeFactory = func(ctx context.Context, offer string) (mediaBridge, string, error) {
			return newWebRTCBridge(ctx, offer)
		}
	}
	if options.offerTimeout <= 0 {
		options.offerTimeout = incomingOfferTTL
	}
	service := &callService{
		registry:         newRegistry(terminalRetention, maxTerminalCalls),
		broker:           newEventBroker(eventBufferSize),
		clientPointer:    options.clientPointer,
		whatsmeowService: options.whatsmeowService,
		loggerWrapper:    options.loggerWrapper,
		engineFactory:    options.engineFactory,
		bridgeFactory:    options.bridgeFactory,
		publishHook:      options.publishHook,
		offerTimeout:     options.offerTimeout,
		runtimes:         make(map[registryKey]*callRuntime),
		instances:        make(map[string]*instance_model.Instance),
	}
	if service.publishHook == nil && service.whatsmeowService != nil {
		service.publishHook = func(instance *instance_model.Instance, event Event) {
			payload, err := json.Marshal(event)
			if err != nil {
				return
			}
			queue := strings.ToLower(instance.Id + "." + event.Type)
			service.whatsmeowService.CallWebhook(instance, queue, payload)
			service.whatsmeowService.SendToGlobalQueues(event.Type, payload, instance.Id)
		}
	}
	return service
}

func NewCallService(
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	loggerWrapper *logger_wrapper.LoggerManager,
) CallService {
	return newCallService(callServiceOptions{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
		loggerWrapper:    loggerWrapper,
	})
}

func (s *callService) Start(ctx context.Context, instance *instance_model.Instance, clientID, number string) (Call, error) {
	if instance == nil {
		return Call{}, ErrCallNotFound
	}
	if clientID == "" {
		return Call{}, ErrClientIDRequired
	}
	peer, err := normalizeCallNumber(number)
	if err != nil {
		return Call{}, err
	}
	client, err := s.connectedClient(instance.Id)
	if err != nil {
		return Call{}, err
	}
	callID := signaling.GenerateCallID()
	call := newOutgoingCall(instance.Id, callID, clientID, peer.String())
	if err := s.registry.add(call); err != nil {
		return Call{}, err
	}
	s.rememberInstance(instance)
	runtime := s.createRuntime(instance.Id, callID, client)
	s.publish(instance, EventStatus, call)
	if err := runtime.engine.Start(ctx, callID, peer); err != nil {
		s.finish(instance.Id, callID, StatusFailed, err.Error())
		return Call{}, err
	}
	return call, nil
}

func (s *callService) NegotiateWebRTC(ctx context.Context, instanceID, callID, clientID, offer string) (string, error) {
	if clientID == "" {
		return "", ErrClientIDRequired
	}
	call, err := s.registry.get(instanceID, callID)
	if err != nil {
		return "", err
	}
	if call.Status.terminal() {
		return "", ErrInvalidTransition
	}
	if call.ClientID != "" && call.ClientID != clientID {
		return "", ErrCallOwned
	}
	runtime := s.runtime(instanceID, callID)
	if runtime == nil {
		return "", ErrCallNotFound
	}
	negotiationCtx, cancel := context.WithTimeout(ctx, negotiationTimeout)
	defer cancel()
	bridge, answer, err := s.bridgeFactory(negotiationCtx, offer)
	if err != nil {
		return "", err
	}
	if err := s.registry.claim(instanceID, callID, clientID); err != nil {
		_ = bridge.Close()
		return "", err
	}
	bridge.SetBrowserPCMHandler(runtime.engine.FeedPCM)
	bridge.SetTerminalHandler(func() {
		go func() { _, _ = s.End(context.Background(), instanceID, callID, clientID) }()
	})
	runtime.replaceBridge(bridge)
	claimed, _ := s.registry.get(instanceID, callID)
	s.publish(s.instance(instanceID), EventStatus, claimed)
	return answer, nil
}

func (s *callService) Accept(ctx context.Context, instanceID, callID, clientID string) error {
	call, err := s.ownedCall(instanceID, callID, clientID)
	if err != nil {
		return err
	}
	if call.Direction != DirectionIncoming || call.Status != StatusOffered {
		return ErrInvalidTransition
	}
	runtime := s.runtime(instanceID, callID)
	if runtime == nil {
		return ErrCallNotFound
	}
	if !runtime.bridgeReady() {
		return ErrWebRTCNotReady
	}
	call, _, err = s.registry.transition(instanceID, callID, StatusStarting)
	if err != nil {
		return err
	}
	s.publish(s.instance(instanceID), EventStatus, call)
	if err := runtime.engine.Accept(ctx, callID); err != nil {
		s.finish(instanceID, callID, StatusFailed, err.Error())
		return err
	}
	runtime.stopOfferTimer()
	return nil
}

func (s *callService) Reject(ctx context.Context, instanceID, callID, clientID string) (Call, error) {
	if clientID == "" {
		return Call{}, ErrClientIDRequired
	}
	call, err := s.registry.get(instanceID, callID)
	if err != nil {
		return Call{}, err
	}
	if call.Status.terminal() {
		return call, nil
	}
	if call.ClientID == "" {
		if err := s.registry.claim(instanceID, callID, clientID); err != nil {
			return Call{}, err
		}
	} else if call.ClientID != clientID {
		return Call{}, ErrCallOwned
	}
	runtime := s.runtime(instanceID, callID)
	if runtime == nil {
		return Call{}, ErrCallNotFound
	}
	if err := runtime.engine.Reject(ctx, callID); err != nil {
		s.finish(instanceID, callID, StatusFailed, err.Error())
		return Call{}, err
	}
	return s.finish(instanceID, callID, StatusRejected, "declined")
}

func (s *callService) End(ctx context.Context, instanceID, callID, clientID string) (Call, error) {
	call, err := s.registry.get(instanceID, callID)
	if err != nil {
		return Call{}, err
	}
	if call.Status.terminal() {
		if clientID != "" && call.ClientID != "" && call.ClientID != clientID {
			return Call{}, ErrCallOwned
		}
		return call, nil
	}
	if _, err := s.ownedCall(instanceID, callID, clientID); err != nil {
		return Call{}, err
	}
	if call.Status != StatusEnding {
		call, _, err = s.registry.transition(instanceID, callID, StatusEnding)
		if err != nil {
			return Call{}, err
		}
		s.publish(s.instance(instanceID), EventStatus, call)
	}
	runtime := s.runtime(instanceID, callID)
	if runtime == nil {
		return s.finish(instanceID, callID, StatusEnded, "local")
	}
	if err := runtime.engine.End(ctx); err != nil {
		s.finish(instanceID, callID, StatusFailed, err.Error())
		return Call{}, err
	}
	return s.finish(instanceID, callID, StatusEnded, "local")
}

func (s *callService) Active(instanceID, clientID string) []Call {
	return s.registry.active(instanceID, clientID)
}

func (s *callService) Subscribe(instanceID, clientID string) *Subscription {
	return s.broker.subscribe(instanceID, clientID)
}

func (s *callService) RejectCall(data *RejectCallStruct, instance *instance_model.Instance) error {
	if data == nil || instance == nil || data.CallID == "" || data.CallCreator.IsEmpty() {
		return ErrInvalidNumber
	}
	if call, err := s.registry.get(instance.Id, data.CallID); err == nil && !call.Status.terminal() {
		runtime := s.runtime(instance.Id, data.CallID)
		if runtime != nil {
			if err := runtime.engine.Reject(context.Background(), data.CallID); err != nil {
				return err
			}
			_, err = s.finish(instance.Id, data.CallID, StatusRejected, "declined")
			return err
		}
	}
	client, err := s.connectedClient(instance.Id)
	if err != nil {
		return err
	}
	return client.RejectCall(context.Background(), data.CallCreator, data.CallID)
}

func (s *callService) HandleWhatsAppEvent(instance *instance_model.Instance, client *whatsmeow.Client, event any) {
	if instance == nil {
		return
	}
	s.rememberInstance(instance)
	switch typed := event.(type) {
	case *events.Disconnected, *events.LoggedOut:
		s.CloseInstance(instance.Id)
		return
	case *events.CallOffer:
		if instance.RejectCall {
			return
		}
		s.handleIncoming(instance, client, typed)
		return
	}
	callID := eventCallID(event)
	if callID == "" {
		return
	}
	runtime := s.runtime(instance.Id, callID)
	if runtime != nil {
		runtime.engine.HandleEvent(context.Background(), event)
	}
}

func (s *callService) CloseInstance(instanceID string) {
	for _, call := range s.registry.instanceCalls(instanceID) {
		runtime := s.runtime(instanceID, call.CallID)
		ended, changed, err := s.registry.terminate(instanceID, call.CallID, StatusFailed, "disconnected")
		if err == nil && changed {
			s.removeRuntime(instanceID, call.CallID)
			s.publish(s.instance(instanceID), EventEnded, ended)
		}
		if runtime != nil {
			_ = runtime.engine.End(context.Background())
		}
	}
}

func (s *callService) handleIncoming(instance *instance_model.Instance, client *whatsmeow.Client, event *events.CallOffer) {
	if event.CallID == "" {
		return
	}
	if _, err := s.registry.get(instance.Id, event.CallID); err == nil {
		return
	}
	peer := event.CallCreator
	if peer.IsEmpty() {
		peer = event.From
	}
	call := newIncomingCall(instance.Id, event.CallID, peer.String())
	if err := s.registry.add(call); err != nil {
		return
	}
	runtime := s.createRuntime(instance.Id, event.CallID, client)
	s.publish(instance, EventIncoming, call)
	runtime.engine.HandleEvent(context.Background(), event)
	runtime.setOfferTimer(time.AfterFunc(s.offerTimeout, func() {
		ended, changed, err := s.registry.terminateIfStatus(
			instance.Id, event.CallID, StatusOffered, StatusRejected, "timeout",
		)
		if err != nil || !changed {
			return
		}
		s.removeRuntime(instance.Id, event.CallID)
		s.publish(instance, EventEnded, ended)
		_ = runtime.engine.Reject(context.Background(), event.CallID)
	}))
}

func (s *callService) createRuntime(instanceID, callID string, client *whatsmeow.Client) *callRuntime {
	engine := s.engineFactory(client)
	runtime := &callRuntime{engine: engine}
	engine.SetCallbacks(engineCallbacks{
		State: func(info *wacall.CallInfo) { s.handleEngineState(instanceID, callID, info) },
		Ended: func(info *wacall.CallInfo) { s.handleEngineState(instanceID, callID, info) },
		PeerPCM: func(pcm []float32) {
			if current := s.runtime(instanceID, callID); current != nil {
				_ = current.writePCM(pcm)
			}
		},
	})
	s.mu.Lock()
	s.runtimes[registryKey{instanceID, callID}] = runtime
	s.mu.Unlock()
	return runtime
}

func (s *callService) handleEngineState(instanceID, callID string, info *wacall.CallInfo) {
	if info == nil {
		return
	}
	current, err := s.registry.get(instanceID, callID)
	if err != nil || current.Status.terminal() {
		return
	}
	next, reason := publicState(info, current.Direction)
	if next.terminal() {
		_, _ = s.finish(instanceID, callID, next, reason)
		return
	}
	if next == current.Status {
		return
	}
	updated, changed, err := s.registry.transition(instanceID, callID, next)
	if err == nil && changed {
		s.publish(s.instance(instanceID), EventStatus, updated)
	}
}

func publicState(info *wacall.CallInfo, direction CallDirection) (CallStatus, string) {
	switch info.StateData.State {
	case wacore.CallStateIncomingRinging:
		return StatusOffered, ""
	case wacore.CallStateInitiating:
		return StatusStarting, ""
	case wacore.CallStateRinging:
		return StatusRinging, ""
	case wacore.CallStateConnecting:
		if direction == DirectionIncoming {
			return StatusStarting, ""
		}
		return StatusRinging, ""
	case wacore.CallStateActive, wacore.CallStateOnHold:
		return StatusConnected, ""
	case wacore.CallStateEnded:
		reason := string(info.StateData.EndReason)
		if info.StateData.EndReason == wacore.EndCallReasonDeclined || info.StateData.EndReason == wacore.EndCallReasonBusy {
			return StatusRejected, reason
		}
		if info.StateData.EndReason == wacore.EndCallReasonFailed {
			return StatusFailed, reason
		}
		return StatusEnded, reason
	default:
		return StatusFailed, "unknown engine state"
	}
}

func (s *callService) finish(instanceID, callID string, terminal CallStatus, reason string) (Call, error) {
	call, changed, err := s.registry.terminate(instanceID, callID, terminal, reason)
	if err != nil {
		return Call{}, err
	}
	if changed {
		s.removeRuntime(instanceID, callID)
		s.publish(s.instance(instanceID), EventEnded, call)
	}
	return call, nil
}

func (s *callService) ownedCall(instanceID, callID, clientID string) (Call, error) {
	if clientID == "" {
		return Call{}, ErrClientIDRequired
	}
	call, err := s.registry.get(instanceID, callID)
	if err != nil {
		return Call{}, err
	}
	if call.ClientID == "" || call.ClientID != clientID {
		return Call{}, ErrCallOwned
	}
	return call, nil
}

func (s *callService) connectedClient(instanceID string) (*whatsmeow.Client, error) {
	if s.clientPointer == nil {
		return nil, ErrClientDisconnected
	}
	client := s.clientPointer[instanceID]
	if client == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return nil, ErrClientDisconnected
	}
	return client, nil
}

func normalizeCallNumber(number string) (types.JID, error) {
	number = strings.TrimSpace(strings.TrimPrefix(number, "+"))
	var digits strings.Builder
	for _, char := range number {
		switch {
		case char >= '0' && char <= '9':
			digits.WriteRune(char)
		case char == ' ', char == '-', char == '(', char == ')':
		default:
			return types.JID{}, ErrInvalidNumber
		}
	}
	value := digits.String()
	if len(value) < 8 || len(value) > 15 {
		return types.JID{}, ErrInvalidNumber
	}
	return types.NewJID(value, types.DefaultUserServer), nil
}

func (s *callService) publish(instance *instance_model.Instance, eventType string, call Call) {
	event := eventFromCall(eventType, call)
	s.broker.publish(event)
	if instance != nil && s.publishHook != nil {
		go s.publishHook(instance, event)
	}
}

func (s *callService) rememberInstance(instance *instance_model.Instance) {
	s.mu.Lock()
	s.instances[instance.Id] = instance
	s.mu.Unlock()
}

func (s *callService) instance(instanceID string) *instance_model.Instance {
	s.mu.RLock()
	instance := s.instances[instanceID]
	s.mu.RUnlock()
	return instance
}

func (s *callService) runtime(instanceID, callID string) *callRuntime {
	s.mu.RLock()
	runtime := s.runtimes[registryKey{instanceID, callID}]
	s.mu.RUnlock()
	return runtime
}

func (s *callService) removeRuntime(instanceID, callID string) {
	key := registryKey{instanceID, callID}
	s.mu.Lock()
	runtime := s.runtimes[key]
	delete(s.runtimes, key)
	s.mu.Unlock()
	if runtime != nil {
		runtime.close()
	}
}

func (r *callRuntime) replaceBridge(bridge mediaBridge) {
	r.mu.Lock()
	old := r.bridge
	r.bridge = bridge
	r.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
}

func (r *callRuntime) bridgeReady() bool {
	r.mu.RLock()
	bridge := r.bridge
	r.mu.RUnlock()
	return bridge != nil && bridge.Ready()
}

func (r *callRuntime) writePCM(pcm []float32) error {
	r.mu.RLock()
	bridge := r.bridge
	r.mu.RUnlock()
	if bridge == nil {
		return ErrWebRTCNotReady
	}
	return bridge.WritePCM(pcm)
}

func (r *callRuntime) setOfferTimer(timer *time.Timer) {
	r.mu.Lock()
	if r.offerTimer != nil {
		r.offerTimer.Stop()
	}
	r.offerTimer = timer
	r.mu.Unlock()
}

func (r *callRuntime) stopOfferTimer() {
	r.mu.Lock()
	if r.offerTimer != nil {
		r.offerTimer.Stop()
		r.offerTimer = nil
	}
	r.mu.Unlock()
}

func (r *callRuntime) close() {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		bridge := r.bridge
		r.bridge = nil
		if r.offerTimer != nil {
			r.offerTimer.Stop()
			r.offerTimer = nil
		}
		r.mu.Unlock()
		if bridge != nil {
			_ = bridge.Close()
		}
	})
}

type whatsAppCallEngine struct {
	manager *wacall.CallManager
}

func newWhatsAppCallEngine(client *whatsmeow.Client) *whatsAppCallEngine {
	return &whatsAppCallEngine{manager: wacall.NewCallManager(waadapter.NewSocket(client), slog.Default())}
}

func (e *whatsAppCallEngine) SetCallbacks(callbacks engineCallbacks) {
	e.manager.OnStateChange = callbacks.State
	e.manager.OnEnded = callbacks.Ended
	e.manager.OnPeerAudio = callbacks.PeerPCM
}

func (e *whatsAppCallEngine) Start(ctx context.Context, callID string, peer types.JID) error {
	return e.manager.StartCall(ctx, callID, peer, false)
}

func (e *whatsAppCallEngine) Accept(ctx context.Context, callID string) error {
	return e.manager.AcceptCall(ctx, callID)
}

func (e *whatsAppCallEngine) Reject(ctx context.Context, callID string) error {
	return e.manager.RejectCall(ctx, callID, wacore.EndCallReasonDeclined)
}

func (e *whatsAppCallEngine) End(ctx context.Context) error {
	return e.manager.EndCall(ctx, wacore.EndCallReasonUserEnded)
}

func (e *whatsAppCallEngine) FeedPCM(pcm []float32) { e.manager.FeedCapturedPCM(pcm) }

func (e *whatsAppCallEngine) HandleEvent(ctx context.Context, event any) {
	switch typed := event.(type) {
	case *events.CallOffer:
		e.manager.HandleCallOffer(ctx, wrapCallNode(typed.From, typed.Data), typed.From)
	case *events.CallAccept:
		e.manager.HandleCallAccept(ctx, wrapCallNode(typed.From, typed.Data), typed.From)
	case *events.CallTransport:
		e.manager.HandleCallTransport(ctx, wrapCallNode(typed.From, typed.Data), typed.From)
	case *events.CallTerminate:
		e.manager.HandleCallTerminate(wrapCallNode(typed.From, typed.Data))
	case *events.CallReject:
		e.manager.HandleCallTerminate(wrapCallNode(typed.From, typed.Data))
	}
}

func wrapCallNode(from types.JID, inner *waBinary.Node) *waBinary.Node {
	content := make([]waBinary.Node, 0, 1)
	if inner != nil {
		content = append(content, *inner)
	}
	return &waBinary.Node{Tag: "call", Attrs: waBinary.Attrs{"from": from}, Content: content}
}

func eventCallID(event any) string {
	switch typed := event.(type) {
	case *events.CallAccept:
		return typed.CallID
	case *events.CallPreAccept:
		return typed.CallID
	case *events.CallTransport:
		return typed.CallID
	case *events.CallTerminate:
		return typed.CallID
	case *events.CallReject:
		return typed.CallID
	default:
		return ""
	}
}

var _ CallService = (*callService)(nil)
var _ callEngine = (*whatsAppCallEngine)(nil)
var _ mediaBridge = (*webRTCBridge)(nil)

func domainError(err error) error {
	if err == nil {
		return nil
	}
	for _, target := range []error{
		ErrCallNotFound, ErrCallOwned, ErrClientBusy, ErrInvalidTransition,
		ErrClientIDRequired, ErrInvalidNumber, ErrInvalidSDP, ErrWebRTCNotReady,
		ErrClientDisconnected, ErrMediaUnavailable, ErrNegotiationTimeout,
	} {
		if errors.Is(err, target) {
			return target
		}
	}
	return fmt.Errorf("call operation failed: %w", err)
}
