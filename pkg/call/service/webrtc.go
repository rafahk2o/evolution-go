package call_service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/EvolutionAPI/evolution-go/internal/wacalls/media"
	"github.com/pion/webrtc/v4"
)

const pcmChannelLabel = "pcm"

const defaultWebRTCUDPPort = 50000

type webRTCNetworkConfig struct {
	publicIP string
	udpPort  int
}

var (
	sharedWebRTCAPIOnce sync.Once
	sharedWebRTCAPI     *webrtc.API
	sharedWebRTCCloser  io.Closer
	sharedWebRTCErr     error
)

func loadWebRTCNetworkConfig() (webRTCNetworkConfig, bool, error) {
	publicIP := strings.TrimSpace(os.Getenv("WEBRTC_PUBLIC_IP"))
	portValue := strings.TrimSpace(os.Getenv("WEBRTC_UDP_PORT"))
	if publicIP == "" && portValue == "" {
		return webRTCNetworkConfig{}, false, nil
	}
	if publicIP == "" {
		return webRTCNetworkConfig{}, false, errors.New("WEBRTC_PUBLIC_IP is required when WEBRTC_UDP_PORT is configured")
	}

	udpPort := defaultWebRTCUDPPort
	if portValue != "" {
		parsedPort, err := strconv.Atoi(portValue)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return webRTCNetworkConfig{}, false, fmt.Errorf("invalid WEBRTC_UDP_PORT %q: use a port from 1 to 65535", portValue)
		}
		udpPort = parsedPort
	}
	return webRTCNetworkConfig{publicIP: publicIP, udpPort: udpPort}, true, nil
}

func defaultWebRTCAPI() (*webrtc.API, error) {
	sharedWebRTCAPIOnce.Do(func() {
		config, enabled, err := loadWebRTCNetworkConfig()
		if err != nil {
			sharedWebRTCErr = err
			return
		}
		if !enabled {
			sharedWebRTCAPI = webrtc.NewAPI()
			return
		}

		packetConn, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", config.udpPort))
		if err != nil {
			sharedWebRTCErr = fmt.Errorf("listen on WebRTC UDP port %d: %w", config.udpPort, err)
			return
		}
		sharedWebRTCAPI, sharedWebRTCCloser, sharedWebRTCErr = newWebRTCAPI(packetConn, config.publicIP)
	})
	return sharedWebRTCAPI, sharedWebRTCErr
}

func newWebRTCAPI(packetConn net.PacketConn, publicIP string) (*webrtc.API, io.Closer, error) {
	if packetConn == nil {
		return nil, nil, errors.New("WebRTC UDP connection is required")
	}
	if ip := net.ParseIP(publicIP); ip == nil || ip.To4() == nil {
		_ = packetConn.Close()
		return nil, nil, fmt.Errorf("invalid WEBRTC_PUBLIC_IP %q: an IPv4 address is required", publicIP)
	}

	udpMux := webrtc.NewICEUDPMux(nil, packetConn)
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetICEUDPMux(udpMux)
	if err := settingEngine.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
		External:        []string{publicIP},
		AsCandidateType: webrtc.ICECandidateTypeHost,
		Mode:            webrtc.ICEAddressRewriteReplace,
	}); err != nil {
		_ = udpMux.Close()
		return nil, nil, fmt.Errorf("configure WebRTC public IP: %w", err)
	}

	return webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine)), udpMux, nil
}

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
	api, err := defaultWebRTCAPI()
	if err != nil {
		return nil, "", fmt.Errorf("configure WebRTC network: %w", err)
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
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
