package call_service

import "time"

type CallStatus string

const (
	StatusOffered   CallStatus = "offered"
	StatusStarting  CallStatus = "starting"
	StatusRinging   CallStatus = "ringing"
	StatusConnected CallStatus = "connected"
	StatusEnding    CallStatus = "ending"
	StatusEnded     CallStatus = "ended"
	StatusRejected  CallStatus = "rejected"
	StatusFailed    CallStatus = "failed"
)

func (s CallStatus) terminal() bool {
	return s == StatusEnded || s == StatusRejected || s == StatusFailed
}

type CallDirection string

const (
	DirectionIncoming CallDirection = "incoming"
	DirectionOutgoing CallDirection = "outgoing"
)

type Call struct {
	InstanceID string        `json:"instanceId"`
	CallID     string        `json:"callId"`
	ClientID   string        `json:"clientId,omitempty"`
	Direction  CallDirection `json:"direction"`
	Status     CallStatus    `json:"status"`
	Peer       string        `json:"peer"`
	Reason     string        `json:"reason,omitempty"`
	CreatedAt  time.Time     `json:"createdAt"`
	UpdatedAt  time.Time     `json:"updatedAt"`
	EndedAt    *time.Time    `json:"endedAt,omitempty"`
}

func newIncomingCall(instanceID, callID, peer string) Call {
	now := time.Now().UTC()
	return Call{
		InstanceID: instanceID,
		CallID:     callID,
		Direction:  DirectionIncoming,
		Status:     StatusOffered,
		Peer:       peer,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func newOutgoingCall(instanceID, callID, clientID, peer string) Call {
	now := time.Now().UTC()
	return Call{
		InstanceID: instanceID,
		CallID:     callID,
		ClientID:   clientID,
		Direction:  DirectionOutgoing,
		Status:     StatusStarting,
		Peer:       peer,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

const (
	EventIncoming = "call.incoming"
	EventStatus   = "call.status"
	EventEnded    = "call.ended"
)

type Event struct {
	Type       string        `json:"type"`
	InstanceID string        `json:"instanceId"`
	CallID     string        `json:"callId"`
	ClientID   string        `json:"clientId,omitempty"`
	Direction  CallDirection `json:"direction,omitempty"`
	Status     CallStatus    `json:"status"`
	Peer       string        `json:"peer,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
	Reason     string        `json:"reason,omitempty"`
}

func eventFromCall(eventType string, call Call) Event {
	return Event{
		Type:       eventType,
		InstanceID: call.InstanceID,
		CallID:     call.CallID,
		ClientID:   call.ClientID,
		Direction:  call.Direction,
		Status:     call.Status,
		Peer:       call.Peer,
		Timestamp:  call.UpdatedAt,
		Reason:     call.Reason,
	}
}
