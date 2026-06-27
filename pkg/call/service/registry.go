package call_service

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type registryKey struct {
	instanceID string
	callID     string
}

type registry struct {
	mu           sync.RWMutex
	calls        map[registryKey]Call
	owners       map[string]map[string]string
	terminalTTL  time.Duration
	maxTerminals int
	now          func() time.Time
}

func newRegistry(terminalTTL time.Duration, maxTerminals int) *registry {
	return &registry{
		calls:        make(map[registryKey]Call),
		owners:       make(map[string]map[string]string),
		terminalTTL:  terminalTTL,
		maxTerminals: maxTerminals,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (r *registry) add(call Call) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	key := registryKey{call.InstanceID, call.CallID}
	if _, exists := r.calls[key]; exists {
		return fmt.Errorf("%w: %s", ErrInvalidTransition, call.CallID)
	}
	if call.ClientID != "" {
		if activeID := r.ownerCallLocked(call.InstanceID, call.ClientID); activeID != "" {
			return fmt.Errorf("%w: %s", ErrClientBusy, activeID)
		}
		r.setOwnerLocked(call.InstanceID, call.ClientID, call.CallID)
	}
	r.calls[key] = call
	return nil
}

func (r *registry) claim(instanceID, callID, clientID string) error {
	if clientID == "" {
		return ErrClientIDRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	key := registryKey{instanceID, callID}
	call, exists := r.calls[key]
	if !exists {
		return ErrCallNotFound
	}
	if call.Status.terminal() {
		return ErrInvalidTransition
	}
	if call.ClientID != "" && call.ClientID != clientID {
		return ErrCallOwned
	}
	if activeID := r.ownerCallLocked(instanceID, clientID); activeID != "" && activeID != callID {
		return ErrClientBusy
	}
	if call.ClientID == "" {
		call.ClientID = clientID
		call.UpdatedAt = r.now()
		r.calls[key] = call
		r.setOwnerLocked(instanceID, clientID, callID)
	}
	return nil
}

func (r *registry) transition(instanceID, callID string, next CallStatus) (Call, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	key := registryKey{instanceID, callID}
	call, exists := r.calls[key]
	if !exists {
		return Call{}, false, ErrCallNotFound
	}
	if call.Status == next {
		return call, false, nil
	}
	if call.Status.terminal() || next.terminal() || !validTransition(call.Status, next) {
		return call, false, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, call.Status, next)
	}
	call.Status = next
	call.UpdatedAt = r.now()
	r.calls[key] = call
	return call, true, nil
}

func validTransition(current, next CallStatus) bool {
	switch current {
	case StatusOffered:
		return next == StatusStarting || next == StatusEnding
	case StatusStarting:
		return next == StatusRinging || next == StatusConnected || next == StatusEnding
	case StatusRinging:
		return next == StatusConnected || next == StatusEnding
	case StatusConnected:
		return next == StatusEnding
	case StatusEnding:
		return false
	default:
		return false
	}
}

func (r *registry) terminate(instanceID, callID string, terminal CallStatus, reason string) (Call, bool, error) {
	if !terminal.terminal() {
		return Call{}, false, ErrInvalidTransition
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	key := registryKey{instanceID, callID}
	call, exists := r.calls[key]
	if !exists {
		return Call{}, false, ErrCallNotFound
	}
	if call.Status.terminal() {
		return call, false, nil
	}
	now := r.now()
	call.Status = terminal
	call.Reason = reason
	call.UpdatedAt = now
	call.EndedAt = &now
	r.calls[key] = call
	r.releaseOwnerLocked(call)
	r.limitTerminalsLocked()
	return call, true, nil
}

func (r *registry) terminateIfStatus(instanceID, callID string, expected, terminal CallStatus, reason string) (Call, bool, error) {
	if !terminal.terminal() {
		return Call{}, false, ErrInvalidTransition
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	key := registryKey{instanceID, callID}
	call, exists := r.calls[key]
	if !exists {
		return Call{}, false, ErrCallNotFound
	}
	if call.Status != expected {
		return call, false, nil
	}
	now := r.now()
	call.Status = terminal
	call.Reason = reason
	call.UpdatedAt = now
	call.EndedAt = &now
	r.calls[key] = call
	r.releaseOwnerLocked(call)
	r.limitTerminalsLocked()
	return call, true, nil
}

func (r *registry) get(instanceID, callID string) (Call, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	call, exists := r.calls[registryKey{instanceID, callID}]
	if !exists {
		return Call{}, ErrCallNotFound
	}
	return call, nil
}

func (r *registry) active(instanceID, clientID string) []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	result := make([]Call, 0)
	for key, call := range r.calls {
		if key.instanceID != instanceID || call.Status.terminal() {
			continue
		}
		if clientID != "" && call.ClientID != "" && call.ClientID != clientID {
			continue
		}
		result = append(result, call)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (r *registry) instanceCalls(instanceID string) []Call {
	return r.active(instanceID, "")
}

func (r *registry) ownerCallLocked(instanceID, clientID string) string {
	if instanceOwners := r.owners[instanceID]; instanceOwners != nil {
		return instanceOwners[clientID]
	}
	return ""
}

func (r *registry) setOwnerLocked(instanceID, clientID, callID string) {
	instanceOwners := r.owners[instanceID]
	if instanceOwners == nil {
		instanceOwners = make(map[string]string)
		r.owners[instanceID] = instanceOwners
	}
	instanceOwners[clientID] = callID
}

func (r *registry) releaseOwnerLocked(call Call) {
	if call.ClientID == "" {
		return
	}
	instanceOwners := r.owners[call.InstanceID]
	if instanceOwners == nil || instanceOwners[call.ClientID] != call.CallID {
		return
	}
	delete(instanceOwners, call.ClientID)
	if len(instanceOwners) == 0 {
		delete(r.owners, call.InstanceID)
	}
}

func (r *registry) pruneLocked() {
	if r.terminalTTL <= 0 {
		return
	}
	cutoff := r.now().Add(-r.terminalTTL)
	for key, call := range r.calls {
		if call.Status.terminal() && call.EndedAt != nil && call.EndedAt.Before(cutoff) {
			delete(r.calls, key)
		}
	}
}

func (r *registry) limitTerminalsLocked() {
	if r.maxTerminals <= 0 {
		return
	}
	type terminalEntry struct {
		key registryKey
		at  time.Time
	}
	terminals := make([]terminalEntry, 0)
	for key, call := range r.calls {
		if call.Status.terminal() && call.EndedAt != nil {
			terminals = append(terminals, terminalEntry{key: key, at: *call.EndedAt})
		}
	}
	if len(terminals) <= r.maxTerminals {
		return
	}
	sort.Slice(terminals, func(i, j int) bool { return terminals[i].at.Before(terminals[j].at) })
	for _, entry := range terminals[:len(terminals)-r.maxTerminals] {
		delete(r.calls, entry.key)
	}
}
