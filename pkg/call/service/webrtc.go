package call_service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/EvolutionAPI/evolution-go/internal/wacalls/media"
	"github.com/pion/webrtc/v4"
)

const pcmChannelLabel = "pcm"

type webRTCBridge struct {
	pc *webrtc.PeerConnection
	dc atomic.Pointer[webrtc.DataChannel]

	mu           sync.RWMutex
	onBrowserPCM func([]float32)
	onTerminal   func()
	ready        chan struct{}
	readyOnce    sync.Once
	closeOnce    sync.Once
	closed       atomic.Bool
}

func newWebRTCBridge(ctx context.Context, offerSDP string) (*webRTCBridge, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if offerSDP == "" {
		return nil, "", ErrInvalidSDP
	}
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return nil, "", fmt.Errorf("create peer connection: %w", err)
	}
	bridge := &webRTCBridge{pc: pc, ready: make(chan struct{})}
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != pcmChannelLabel || bridge.closed.Load() {
			_ = dc.Close()
			return
		}
		bridge.dc.Store(dc)
		dc.OnOpen(func() { bridge.readyOnce.Do(func() { close(bridge.ready) }) })
		dc.OnMessage(func(message webrtc.DataChannelMessage) {
			if len(message.Data) == 0 {
				return
			}
			bridge.mu.RLock()
			handler := bridge.onBrowserPCM
			bridge.mu.RUnlock()
			if handler != nil {
				handler(media.PCMInt16LEToFloat32(message.Data))
			}
		})
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state != webrtc.ICEConnectionStateFailed && state != webrtc.ICEConnectionStateClosed {
			return
		}
		bridge.mu.RLock()
		handler := bridge.onTerminal
		bridge.mu.RUnlock()
		if handler != nil {
			handler()
		}
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		bridge.Close()
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidSDP, err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		bridge.Close()
		return nil, "", fmt.Errorf("create SDP answer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		bridge.Close()
		return nil, "", fmt.Errorf("set SDP answer: %w", err)
	}
	select {
	case <-gatherComplete:
	case <-ctx.Done():
		bridge.Close()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, "", fmt.Errorf("%w: %v", ErrNegotiationTimeout, ctx.Err())
		}
		return nil, "", ctx.Err()
	}
	local := pc.LocalDescription()
	if local == nil || local.SDP == "" {
		bridge.Close()
		return nil, "", ErrInvalidSDP
	}
	return bridge, local.SDP, nil
}

func (b *webRTCBridge) SetBrowserPCMHandler(handler func([]float32)) {
	b.mu.Lock()
	b.onBrowserPCM = handler
	b.mu.Unlock()
}

func (b *webRTCBridge) SetTerminalHandler(handler func()) {
	b.mu.Lock()
	b.onTerminal = handler
	b.mu.Unlock()
}

func (b *webRTCBridge) Ready() bool {
	select {
	case <-b.ready:
		return true
	default:
		return false
	}
}

func (b *webRTCBridge) WritePCM(pcm []float32) error {
	if len(pcm) == 0 {
		return nil
	}
	dc := b.dc.Load()
	if dc == nil || dc.ReadyState() != webrtc.DataChannelStateOpen {
		return ErrWebRTCNotReady
	}
	return dc.Send(media.PCMFloat32ToInt16LE(pcm))
}

func (b *webRTCBridge) Close() error {
	var closeErr error
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		if dc := b.dc.Load(); dc != nil {
			_ = dc.Close()
		}
		if b.pc != nil {
			closeErr = b.pc.Close()
		}
	})
	return closeErr
}
