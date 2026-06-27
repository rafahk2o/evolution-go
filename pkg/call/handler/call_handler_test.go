package call_handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	call_service "github.com/EvolutionAPI/evolution-go/pkg/call/service"
	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"
)

type fakeHTTPCallService struct {
	startCall call_service.Call
	startErr  error
	legacyErr error
}

func (f *fakeHTTPCallService) Start(context.Context, *instance_model.Instance, string, string) (call_service.Call, error) {
	return f.startCall, f.startErr
}
func (f *fakeHTTPCallService) NegotiateWebRTC(context.Context, string, string, string, string) (string, error) {
	return "answer", nil
}
func (f *fakeHTTPCallService) Accept(context.Context, string, string, string) error { return nil }
func (f *fakeHTTPCallService) Reject(context.Context, string, string, string) (call_service.Call, error) {
	return call_service.Call{}, nil
}
func (f *fakeHTTPCallService) End(context.Context, string, string, string) (call_service.Call, error) {
	return call_service.Call{}, nil
}
func (f *fakeHTTPCallService) Active(string, string) []call_service.Call           { return nil }
func (f *fakeHTTPCallService) Subscribe(string, string) *call_service.Subscription { return nil }
func (f *fakeHTTPCallService) RejectCall(*call_service.RejectCallStruct, *instance_model.Instance) error {
	return f.legacyErr
}
func (f *fakeHTTPCallService) HandleWhatsAppEvent(*instance_model.Instance, *whatsmeow.Client, any) {}
func (f *fakeHTTPCallService) CloseInstance(string)                                                 {}

func callContext(t *testing.T, method, target, body, clientID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if clientID != "" {
		ctx.Request.Header.Set(callClientIDHeader, clientID)
	}
	ctx.Set("instance", &instance_model.Instance{Id: "instance"})
	return ctx, recorder
}

func TestStartRequiresCallClientID(t *testing.T) {
	handler := NewCallHandler(&fakeHTTPCallService{})
	ctx, response := callContext(t, http.MethodPost, "/call/start", `{"number":"5511999999999"}`, "")
	handler.Start(ctx)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStartMapsOwnershipConflictTo409(t *testing.T) {
	handler := NewCallHandler(&fakeHTTPCallService{startErr: call_service.ErrClientBusy})
	ctx, response := callContext(t, http.MethodPost, "/call/start", `{"number":"5511999999999"}`, "client")
	handler.Start(ctx)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStartReturnsCreatedContract(t *testing.T) {
	handler := NewCallHandler(&fakeHTTPCallService{startCall: call_service.Call{
		CallID: "call", Direction: call_service.DirectionOutgoing, Status: call_service.StatusStarting,
	}})
	ctx, response := callContext(t, http.MethodPost, "/call/start", `{"number":"5511999999999"}`, "client")
	handler.Start(ctx)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["callId"] != "call" || body["status"] != "starting" || len(body) != 3 {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestLegacyRejectDoesNotRequireClientID(t *testing.T) {
	handler := NewCallHandler(&fakeHTTPCallService{})
	ctx, response := callContext(t, http.MethodPost, "/call/reject", `{"callCreator":"5511999999999@s.whatsapp.net","callId":"call"}`, "")
	handler.RejectCall(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDomainErrorStatus(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{call_service.ErrCallNotFound, http.StatusNotFound},
		{call_service.ErrCallOwned, http.StatusConflict},
		{call_service.ErrInvalidSDP, http.StatusUnprocessableEntity},
		{call_service.ErrClientDisconnected, http.StatusServiceUnavailable},
		{call_service.ErrNegotiationTimeout, http.StatusGatewayTimeout},
		{errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		if got := statusForError(test.err); got != test.want {
			t.Errorf("error=%v status=%d want=%d", test.err, got, test.want)
		}
	}
}
