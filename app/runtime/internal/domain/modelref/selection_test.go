package modelref

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		wantSet  bool
		wantErr  error
	}{
		{name: "unset"},
		{name: "configured", provider: "openai", model: "gpt-5", wantSet: true},
		{name: "provider without model", provider: "openai", wantErr: ErrIncomplete},
		{name: "model without provider", model: "gpt-5", wantErr: ErrIncomplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := New(tt.provider, tt.model)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tt.wantErr)
			}
			if selection.Configured() != tt.wantSet {
				t.Fatalf("Configured() = %t, want %t", selection.Configured(), tt.wantSet)
			}
			if selection.Provider() != tt.provider && err == nil {
				t.Fatalf("Provider() = %q, want %q", selection.Provider(), tt.provider)
			}
			if selection.Model() != tt.model && err == nil {
				t.Fatalf("Model() = %q, want %q", selection.Model(), tt.model)
			}
		})
	}
}
