package call_service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func makeBrowserOffer(t *testing.T) (*webrtc.PeerConnection, *webrtc.DataChannel, string) {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	dc, err := pc.CreateDataChannel(pcmChannelLabel, nil)
	if err != nil {
		t.Fatal(err)
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gather := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gather
	return pc, dc, pc.LocalDescription().SDP
}

func TestWebRTCBridgeNegotiatesPCMDataChannel(t *testing.T) {
	offerer, _, offer := makeBrowserOffer(t)
	defer offerer.Close()
	bridge, answer, err := newWebRTCBridge(context.Background(), offer)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	if !strings.Contains(answer, "m=application") || strings.Contains(answer, "m=audio") {
		t.Fatalf("unexpected SDP answer: %s", answer)
	}
	if err := offerer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		t.Fatal(err)
	}
}

func TestWebRTCBridgeReceivesBrowserPCM(t *testing.T) {
	offerer, dc, offer := makeBrowserOffer(t)
	defer offerer.Close()
	bridge, answer, err := newWebRTCBridge(context.Background(), offer)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()

	received := make(chan []float32, 1)
	bridge.SetBrowserPCMHandler(func(pcm []float32) { received <- pcm })
	if err := offerer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
		t.Fatal(err)
	}
	dc.OnOpen(func() { _ = dc.Send([]byte{0x00, 0x40, 0x00, 0xC0}) })

	select {
	case pcm := <-received:
		if len(pcm) != 2 || pcm[0] < 0.49 || pcm[1] > -0.49 {
			t.Fatalf("unexpected PCM: %v", pcm)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for PCM")
	}
}

func TestWebRTCBridgeRejectsInvalidSDP(t *testing.T) {
	_, _, err := newWebRTCBridge(context.Background(), "invalid")
	if !errors.Is(err, ErrInvalidSDP) {
		t.Fatalf("expected ErrInvalidSDP, got %v", err)
	}
}
