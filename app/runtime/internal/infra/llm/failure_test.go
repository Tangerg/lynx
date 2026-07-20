package llm

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	openaisdk "github.com/openai/openai-go/v3"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/core/chat"
)

type callOnlyModel struct{}

func (callOnlyModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return new(chat.Response), nil
}

func TestClassifyModelFailuresPreservesOptionalStreamingCapability(t *testing.T) {
	classified := classifyModelFailures(callOnlyModel{})
	if _, ok := classified.(chat.Streamer); ok {
		t.Fatal("call-only model unexpectedly gained streaming capability")
	}
}

func TestClassifyModelErrorUsesTypedProviderStatus(t *testing.T) {
	providerErr := &openaisdk.Error{
		StatusCode: http.StatusTooManyRequests,
		Response:   &http.Response{Header: http.Header{"Retry-After": []string{"12"}}},
	}
	err := classifyModelError(providerErr)
	var failure *execution.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("classified error = %T, want *execution.Failure", err)
	}
	if failure.Kind != execution.FailureRateLimited || failure.RetryAfter != 12*time.Second {
		t.Fatalf("failure = %+v, want rate limited with 12s retry", failure)
	}
	if !errors.Is(err, providerErr) {
		t.Fatal("classification lost the provider error chain")
	}
}

func TestClassifyModelErrorPreservesCancellationAndClassifiesDeadline(t *testing.T) {
	if got := classifyModelError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("cancellation = %v", got)
	}
	var failure *execution.Failure
	if err := classifyModelError(context.DeadlineExceeded); !errors.As(err, &failure) || failure.Kind != execution.FailureTimeout {
		t.Fatalf("deadline = %#v, want timeout failure", err)
	}
}

func TestFailureKindForHTTPStatus(t *testing.T) {
	cases := []struct {
		status int
		want   execution.FailureKind
	}{
		{http.StatusUnauthorized, execution.FailureInvalidCredentials},
		{http.StatusForbidden, execution.FailureInvalidCredentials},
		{http.StatusRequestTimeout, execution.FailureTimeout},
		{http.StatusTooManyRequests, execution.FailureRateLimited},
		{http.StatusBadRequest, execution.FailureProviderRejected},
		{http.StatusServiceUnavailable, execution.FailureProviderUnavailable},
	}
	for _, test := range cases {
		if got := failureKindForHTTPStatus(test.status); got != test.want {
			t.Errorf("status %d = %d, want %d", test.status, got, test.want)
		}
	}
}
