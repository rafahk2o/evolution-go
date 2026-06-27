package call_service

import "errors"

var (
	ErrCallNotFound       = errors.New("call not found")
	ErrCallOwned          = errors.New("call is owned by another client")
	ErrClientBusy         = errors.New("client already owns an active call")
	ErrInvalidTransition  = errors.New("invalid call state transition")
	ErrClientIDRequired   = errors.New("X-Call-Client-ID is required")
	ErrInvalidNumber      = errors.New("invalid WhatsApp number")
	ErrInvalidSDP         = errors.New("invalid SDP offer")
	ErrWebRTCNotReady     = errors.New("WebRTC bridge is not ready")
	ErrClientDisconnected = errors.New("WhatsApp client is disconnected")
	ErrMediaUnavailable   = errors.New("call media is unavailable")
	ErrNegotiationTimeout = errors.New("WebRTC negotiation timed out")
)
