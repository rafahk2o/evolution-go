package call_handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	call_service "github.com/EvolutionAPI/evolution-go/pkg/call/service"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

const callClientIDHeader = "X-Call-Client-ID"

type CallHandler interface {
	Start(ctx *gin.Context)
	WebRTC(ctx *gin.Context)
	Accept(ctx *gin.Context)
	Reject(ctx *gin.Context)
	End(ctx *gin.Context)
	Active(ctx *gin.Context)
	Events(ctx *gin.Context)
	RejectCall(ctx *gin.Context)
}

type callHandler struct {
	callService call_service.CallService
}

type startRequest struct {
	Number string `json:"number" binding:"required"`
}

type webRTCRequest struct {
	SDPOffer string `json:"sdpOffer" binding:"required"`
}

// Start starts an outgoing WhatsApp voice call.
// @Summary Start a WhatsApp voice call
// @Tags Call
// @Accept json
// @Produce json
// @Param X-Call-Client-ID header string true "Call client identifier"
// @Param request body startRequest true "Destination"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} gin.H
// @Failure 409 {object} gin.H
// @Failure 422 {object} gin.H
// @Failure 503 {object} gin.H
// @Router /call/start [post]
func (h *callHandler) Start(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	clientID, ok := requiredClientID(ctx)
	if !ok {
		return
	}
	var request startRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeError(ctx, fmt.Errorf("invalid request: %w", err))
		return
	}
	call, err := h.callService.Start(ctx.Request.Context(), instance, clientID, request.Number)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{
		"callId": call.CallID, "direction": call.Direction, "status": call.Status,
	})
}

// WebRTC negotiates the browser PCM data channel.
// @Summary Negotiate call WebRTC
// @Tags Call
// @Accept json
// @Produce json
// @Param callId path string true "Call ID"
// @Param X-Call-Client-ID header string true "Call client identifier"
// @Param request body webRTCRequest true "SDP offer"
// @Success 200 {object} map[string]string
// @Router /call/{callId}/webrtc [post]
func (h *callHandler) WebRTC(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	clientID, ok := requiredClientID(ctx)
	if !ok {
		return
	}
	var request webRTCRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeError(ctx, fmt.Errorf("invalid request: %w", err))
		return
	}
	answer, err := h.callService.NegotiateWebRTC(
		ctx.Request.Context(), instance.Id, ctx.Param("callId"), clientID, request.SDPOffer,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"sdpAnswer": answer})
}

// Accept accepts an incoming call after WebRTC is ready.
// @Summary Accept an incoming call
// @Tags Call
// @Param callId path string true "Call ID"
// @Param X-Call-Client-ID header string true "Call client identifier"
// @Success 200 {object} map[string]string
// @Router /call/{callId}/accept [post]
func (h *callHandler) Accept(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	clientID, ok := requiredClientID(ctx)
	if !ok {
		return
	}
	if err := h.callService.Accept(ctx.Request.Context(), instance.Id, ctx.Param("callId"), clientID); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": call_service.StatusStarting})
}

// Reject rejects an incoming call owned by the caller.
// @Summary Reject an incoming call
// @Tags Call
// @Param callId path string true "Call ID"
// @Param X-Call-Client-ID header string true "Call client identifier"
// @Success 200 {object} call_service.Call
// @Router /call/{callId}/reject [post]
func (h *callHandler) Reject(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	clientID, ok := requiredClientID(ctx)
	if !ok {
		return
	}
	call, err := h.callService.Reject(ctx.Request.Context(), instance.Id, ctx.Param("callId"), clientID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, call)
}

// End terminates a call and is idempotent during terminal retention.
// @Summary End a call
// @Tags Call
// @Param callId path string true "Call ID"
// @Param X-Call-Client-ID header string true "Call client identifier"
// @Success 200 {object} call_service.Call
// @Router /call/{callId} [delete]
func (h *callHandler) End(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	clientID, ok := requiredClientID(ctx)
	if !ok {
		return
	}
	call, err := h.callService.End(ctx.Request.Context(), instance.Id, ctx.Param("callId"), clientID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, call)
}

// Active lists active calls for the authenticated instance.
// @Summary List active calls
// @Tags Call
// @Produce json
// @Param X-Call-Client-ID header string false "Optional call client filter"
// @Success 200 {object} map[string]interface{}
// @Router /call/active [get]
func (h *callHandler) Active(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"calls": h.callService.Active(instance.Id, clientID(ctx))})
}

// Events streams normalized call events for the authenticated instance.
// @Summary Stream call events
// @Tags Call
// @Produce text/event-stream
// @Param X-Call-Client-ID header string false "Optional call client filter"
// @Success 200 {string} string
// @Router /call/events [get]
func (h *callHandler) Events(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	flusher, ok := ctx.Writer.(http.Flusher)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "streaming is unavailable"})
		return
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	subscription := h.callService.Subscribe(instance.Id, clientID(ctx))
	if subscription == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "event stream is unavailable"})
		return
	}
	defer subscription.Close()
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Request.Context().Done():
			return
		case event, open := <-subscription.Events():
			if !open {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(ctx.Writer, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			if _, err := ctx.Writer.WriteString(": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// RejectCall preserves the legacy call rejection contract.
// @Summary Reject call (legacy)
// @Description Reject a WhatsApp call using callCreator and callId.
// @Tags Call
// @Accept json
// @Produce json
// @Param message body call_service.RejectCallStruct true "Call data"
// @Success 200 {object} gin.H
// @Router /call/reject [post]
func (h *callHandler) RejectCall(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}
	var request call_service.RejectCallStruct
	if err := ctx.ShouldBindJSON(&request); err != nil {
		writeError(ctx, fmt.Errorf("invalid request: %w", err))
		return
	}
	if err := h.callService.RejectCall(&request, instance); err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func instanceFromContext(ctx *gin.Context) (*instance_model.Instance, bool) {
	value, exists := ctx.Get("instance")
	if !exists {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return nil, false
	}
	instance, ok := value.(*instance_model.Instance)
	if !ok || instance == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return nil, false
	}
	return instance, true
}

func clientID(ctx *gin.Context) string {
	return strings.TrimSpace(ctx.GetHeader(callClientIDHeader))
}

func requiredClientID(ctx *gin.Context) (string, bool) {
	id := clientID(ctx)
	if id == "" {
		writeError(ctx, call_service.ErrClientIDRequired)
		return "", false
	}
	return id, true
}

func writeError(ctx *gin.Context, err error) {
	ctx.JSON(statusForError(err), gin.H{"error": err.Error()})
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, call_service.ErrCallNotFound):
		return http.StatusNotFound
	case errors.Is(err, call_service.ErrCallOwned),
		errors.Is(err, call_service.ErrClientBusy),
		errors.Is(err, call_service.ErrInvalidTransition),
		errors.Is(err, call_service.ErrWebRTCNotReady):
		return http.StatusConflict
	case errors.Is(err, call_service.ErrInvalidNumber),
		errors.Is(err, call_service.ErrInvalidSDP):
		return http.StatusUnprocessableEntity
	case errors.Is(err, call_service.ErrClientDisconnected),
		errors.Is(err, call_service.ErrMediaUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, call_service.ErrNegotiationTimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, call_service.ErrClientIDRequired), strings.HasPrefix(err.Error(), "invalid request:"):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func NewCallHandler(callService call_service.CallService) CallHandler {
	return &callHandler{callService: callService}
}
