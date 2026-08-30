package moderation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	coremoderation "github.com/Tangerg/scope/core/moderation"

	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// TestErrorTypeAttributeStaysLowCardinality is the whole point of this
// classification: the error.type attribute is a metric dimension, so every
// failure has to collapse into a small fixed vocabulary. A provider message
// leaking through would create one time series per distinct error string.
func TestErrorTypeAttributeStaysLowCardinality(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"canceled": {
			err:  fmt.Errorf("provider: %w", context.Canceled),
			want: errorCanceled,
		},
		"deadline exceeded": {
			err:  fmt.Errorf("provider: %w", context.DeadlineExceeded),
			want: errorDeadline,
		},
		"invalid request": {
			err:  fmt.Errorf("provider: %w", coremoderation.ErrInvalidRequest),
			want: errorInvalidRequest,
		},
		"invalid options": {
			err:  fmt.Errorf("provider: %w", coremoderation.ErrInvalidOptions),
			want: errorInvalidRequest,
		},
		"invalid response": {
			err:  fmt.Errorf("provider: %w", coremoderation.ErrInvalidResponse),
			want: errorInvalidOutput,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			attribute := errorTypeAttribute(testCase.err)
			if attribute.Key != semconv.ErrorTypeKey {
				t.Fatalf("attribute key = %q, want %q", attribute.Key, semconv.ErrorTypeKey)
			}
			if got := attribute.Value.AsString(); got != testCase.want {
				t.Fatalf("error type = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestUnclassifiedErrorsFallBackToTheirType keeps an unknown failure out of the
// metric as a message: semconv derives a type name, never the error text.
func TestUnclassifiedErrorsFallBackToTheirType(t *testing.T) {
	attribute := errorTypeAttribute(errors.New("a very specific provider message with an id 12345"))
	if got := attribute.Value.AsString(); got == "a very specific provider message with an id 12345" {
		t.Fatal("the error message became the metric dimension")
	}
}
