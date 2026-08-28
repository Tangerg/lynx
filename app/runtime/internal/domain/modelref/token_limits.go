package modelref

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidTokenLimits reports a negative limit or an independent input or
	// output maximum that exceeds a known total context window.
	ErrInvalidTokenLimits = errors.New("model token limits: invalid")
	// ErrOutputTokenLimitExceeded reports an explicit generation request above
	// the selected model's published output maximum.
	ErrOutputTokenLimitExceeded = errors.New("model token limits: output maximum exceeded")
	// ErrOutputReservationExhaustsContext reports an explicit output reservation
	// that leaves no room for the non-empty model input.
	ErrOutputReservationExhaustsContext = errors.New("model token limits: output reservation exhausts context")
)

// TokenLimits is the immutable provider-published context envelope for one
// exact model. Each zero field means unknown, never unlimited. Input and output
// maxima are independent: both may be individually valid without being usable
// together at their maxima inside the total context window.
type TokenLimits struct {
	contextWindow   int64
	maxInputTokens  int64
	maxOutputTokens int64
}

// NewTokenLimits validates and freezes one model's published token limits.
func NewTokenLimits(contextWindow, maxInputTokens, maxOutputTokens int64) (TokenLimits, error) {
	limits := TokenLimits{
		contextWindow:   contextWindow,
		maxInputTokens:  maxInputTokens,
		maxOutputTokens: maxOutputTokens,
	}
	if err := limits.Validate(); err != nil {
		return TokenLimits{}, err
	}
	return limits, nil
}

// Validate checks the relationships that are knowable without inventing
// provider defaults. A zero total window leaves the independent maxima usable
// as facts, while a known total window bounds each maximum individually.
func (t TokenLimits) Validate() error {
	if t.contextWindow < 0 || t.maxInputTokens < 0 || t.maxOutputTokens < 0 {
		return fmt.Errorf("%w: values must not be negative", ErrInvalidTokenLimits)
	}
	if t.contextWindow > 0 && t.maxInputTokens > t.contextWindow {
		return fmt.Errorf(
			"%w: max input %d exceeds context window %d",
			ErrInvalidTokenLimits,
			t.maxInputTokens,
			t.contextWindow,
		)
	}
	if t.contextWindow > 0 && t.maxOutputTokens > t.contextWindow {
		return fmt.Errorf(
			"%w: max output %d exceeds context window %d",
			ErrInvalidTokenLimits,
			t.maxOutputTokens,
			t.contextWindow,
		)
	}
	return nil
}

// IsZero reports that the provider published no context-limit facts.
func (t TokenLimits) IsZero() bool { return t == TokenLimits{} }

// ContextWindow returns the published total input-plus-output window.
func (t TokenLimits) ContextWindow() int64 { return t.contextWindow }

// MaxInputTokens returns the published independent prompt maximum.
func (t TokenLimits) MaxInputTokens() int64 { return t.maxInputTokens }

// MaxOutputTokens returns the published independent generation maximum.
func (t TokenLimits) MaxOutputTokens() int64 { return t.maxOutputTokens }

// InputCeiling returns the hard prompt ceiling after reserving an explicitly
// requested output. requestedOutput == 0 means the caller did not request a
// generation ceiling, so this value does not guess a provider default.
// The bool is false only when neither the provider input maximum nor a total
// context reservation establishes a hard input ceiling.
func (t TokenLimits) InputCeiling(requestedOutput int64) (int64, bool, error) {
	if err := t.Validate(); err != nil {
		return 0, false, err
	}
	if requestedOutput < 0 {
		return 0, false, fmt.Errorf(
			"%w: requested output %d must not be negative",
			ErrInvalidTokenLimits,
			requestedOutput,
		)
	}
	if requestedOutput > 0 && t.maxOutputTokens > 0 && requestedOutput > t.maxOutputTokens {
		return 0, false, fmt.Errorf(
			"%w: requested %d, maximum %d",
			ErrOutputTokenLimitExceeded,
			requestedOutput,
			t.maxOutputTokens,
		)
	}

	ceiling := t.maxInputTokens
	if requestedOutput > 0 && t.contextWindow > 0 {
		if requestedOutput >= t.contextWindow {
			return 0, false, fmt.Errorf(
				"%w: requested %d, context window %d",
				ErrOutputReservationExhaustsContext,
				requestedOutput,
				t.contextWindow,
			)
		}
		remaining := t.contextWindow - requestedOutput
		if ceiling == 0 || remaining < ceiling {
			ceiling = remaining
		}
	}
	return ceiling, ceiling > 0, nil
}
