package modelrole

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		model      string
		wantSet    bool
		wantErr    error
	}{
		{name: "configured", providerID: "openai", model: "gpt-5", wantSet: true},
		{name: "unset"},
		{name: "provider requires model", providerID: "openai", wantErr: modelref.ErrIncomplete},
		{name: "model requires provider", model: "gpt-5", wantErr: modelref.ErrIncomplete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.providerID, tt.model)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tt.wantErr)
			}
			if got.Configured() != tt.wantSet {
				t.Fatalf("Configured() = %t, want %t", got.Configured(), tt.wantSet)
			}
			if got.ProviderID() != tt.providerID && tt.wantErr == nil {
				t.Fatalf("ProviderID() = %q, want %q", got.ProviderID(), tt.providerID)
			}
			if got.Model() != tt.model && tt.wantErr == nil {
				t.Fatalf("Model() = %q, want %q", got.Model(), tt.model)
			}
		})
	}
}
