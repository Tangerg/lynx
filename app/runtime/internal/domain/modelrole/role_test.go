package modelrole

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		model      string
		want       Role
		wantErr    error
	}{
		{name: "configured", providerID: "openai", model: "gpt-5", want: Role{providerID: "openai", model: "gpt-5"}},
		{name: "unset", want: Role{}},
		{name: "empty model clears provider", providerID: "openai", want: Role{}},
		{name: "model requires provider", model: "gpt-5", wantErr: ErrProviderRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.providerID, tt.model)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("New() = %+v, want %+v", got, tt.want)
			}
			if got.Configured() != (tt.want.model != "") {
				t.Fatalf("Configured() = %t", got.Configured())
			}
		})
	}
}
