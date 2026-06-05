package session

import "testing"

func TestSessionEffectiveModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		fallback string
		want     string
	}{
		{"explicit model wins", "claude-opus-4-8", "gpt-5", "claude-opus-4-8"},
		{"empty falls back", "", "gpt-5", "gpt-5"},
		{"empty and empty fallback stays empty", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := Session{Model: tc.model}
			if got := s.EffectiveModel(tc.fallback); got != tc.want {
				t.Errorf("EffectiveModel(%q) = %q, want %q", tc.fallback, got, tc.want)
			}
		})
	}
}
