package gateway

import (
	"errors"
	"sync"
)

type AccountState string

const (
	StateActive    AccountState = "active"
	StateSuspended AccountState = "suspended"
)

var (
	ErrTenantUnknown   = errors.New("tenant not found")
	ErrTenantExists    = errors.New("tenant already exists")
	ErrAccountInactive = errors.New("tenant account is not active")
)

type TenantRegistry struct {
	mu     sync.RWMutex
	states map[string]AccountState
}

func NewTenantRegistry() *TenantRegistry {
	return &TenantRegistry{states: make(map[string]AccountState)}
}

func (r *TenantRegistry) Onboard(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.states[id]; exists {
		return ErrTenantExists
	}
	r.states[id] = StateActive
	return nil
}

func (r *TenantRegistry) SetState(id string, state AccountState) error {
	if state != StateActive && state != StateSuspended {
		return errors.New("invalid account state")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.states[id]; !exists {
		return ErrTenantUnknown
	}
	r.states[id] = state
	return nil
}

func (r *TenantRegistry) AuthorizeCompression(id string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, exists := r.states[id]
	if !exists {
		return ErrTenantUnknown
	}
	if state != StateActive {
		return ErrAccountInactive
	}
	return nil
}
