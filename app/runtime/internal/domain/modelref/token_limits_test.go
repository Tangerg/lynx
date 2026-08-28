package modelref

import (
	"errors"
	"testing"
)

func TestTokenLimitsInputCeilingOwnsIndependentProviderLimits(t *testing.T) {
	tests := []struct {
		name            string
		contextWindow   int64
		maxInputTokens  int64
		maxOutputTokens int64
		requestedOutput int64
		wantCeiling     int64
		wantKnown       bool
		wantErr         error
	}{
		{name: "unknown"},
		{
			name: "provider input maximum", contextWindow: 400_000,
			maxInputTokens: 272_000, maxOutputTokens: 128_000,
			wantCeiling: 272_000, wantKnown: true,
		},
		{
			name: "explicit output reservation", contextWindow: 16_384,
			maxOutputTokens: 8_192, requestedOutput: 8_192,
			wantCeiling: 8_192, wantKnown: true,
		},
		{
			name: "reservation tighter than independent input maximum", contextWindow: 400_000,
			maxInputTokens: 272_000, maxOutputTokens: 272_000, requestedOutput: 272_000,
			wantCeiling: 128_000, wantKnown: true,
		},
		{
			name: "requested output above provider maximum", contextWindow: 400_000,
			maxInputTokens: 272_000, maxOutputTokens: 128_000, requestedOutput: 128_001,
			wantErr: ErrOutputTokenLimitExceeded,
		},
		{
			name: "requested output consumes total context", contextWindow: 16_384,
			requestedOutput: 16_384, wantErr: ErrOutputReservationExhaustsContext,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits, err := NewTokenLimits(tt.contextWindow, tt.maxInputTokens, tt.maxOutputTokens)
			if err != nil {
				t.Fatal(err)
			}
			ceiling, known, err := limits.InputCeiling(tt.requestedOutput)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("InputCeiling() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (ceiling != tt.wantCeiling || known != tt.wantKnown) {
				t.Fatalf(
					"InputCeiling() = (%d,%t), want (%d,%t)",
					ceiling,
					known,
					tt.wantCeiling,
					tt.wantKnown,
				)
			}
		})
	}
}

func TestTokenLimitsRejectsImpossiblePublishedMaximum(t *testing.T) {
	_, err := NewTokenLimits(1_000, 1_001, 0)
	if !errors.Is(err, ErrInvalidTokenLimits) {
		t.Fatalf("NewTokenLimits() error = %v, want ErrInvalidTokenLimits", err)
	}
}
