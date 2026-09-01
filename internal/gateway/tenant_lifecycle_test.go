package gateway

import (
	"errors"
	"testing"
)

func TestTenantCompressionDecision(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		setup     func(*TenantRegistry)
		wantError error
	}{
		{"onboarded tenant is accepted", "acme", func(r *TenantRegistry) { mustOnboard(t, r, "acme") }, nil},
		{"suspended tenant is rejected", "acme", func(r *TenantRegistry) {
			mustOnboard(t, r, "acme")
			if err := r.SetState("acme", StateSuspended); err != nil {
				t.Fatal(err)
			}
		}, ErrAccountInactive},
		{"reactivated tenant is accepted", "acme", func(r *TenantRegistry) {
			mustOnboard(t, r, "acme")
			if err := r.SetState("acme", StateSuspended); err != nil {
				t.Fatal(err)
			}
			if err := r.SetState("acme", StateActive); err != nil {
				t.Fatal(err)
			}
		}, nil},
		{"unknown tenant is rejected", "missing", func(*TenantRegistry) {}, ErrTenantUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewTenantRegistry()
			tt.setup(registry)
			err := registry.AuthorizeCompression(tt.tenantID)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("AuthorizeCompression() error = %v, want %v", err, tt.wantError)
			}
		})
	}
}

func mustOnboard(t *testing.T, registry *TenantRegistry, id string) {
	t.Helper()
	if err := registry.Onboard(id); err != nil {
		t.Fatal(err)
	}
}
