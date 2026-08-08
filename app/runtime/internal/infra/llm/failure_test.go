package llm

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/core/chat"
)

type callOnlyModel struct{}

func (callOnlyModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return new(chat.Response), nil
}

type providerError struct {
	status int
	header http.Header
}

func (err *providerError) Error() string           { return "provider error" }
func (err *providerError) HTTPStatus() int         { return err.status }
func (err *providerError) HTTPHeader() http.Header { return err.header }

func TestClassifyModelFailuresPreservesOptionalStreamingCapability(t *testing.T) {
	classified := classifyModelFailures(callOnlyModel{})
	if _, ok := classified.(chat.Streamer); ok {
		t.Fatal("call-only model unexpectedly gained streaming capability")
	}
}

func TestClassifyModelErrorUsesTypedProviderStatus(t *testing.T) {
	providerErr := &providerError{
		status: http.StatusTooManyRequests,
		header: http.Header{"Retry-After": []string{"12"}},
	}
	err := classifyModelError(providerErr)
	var failure *run.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("classified error = %T, want *run.Failure", err)
	}
	if failure.Kind != run.FailureRateLimited || failure.RetryAfter != 12*time.Second {
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
	var failure *run.Failure
	if err := classifyModelError(context.DeadlineExceeded); !errors.As(err, &failure) || failure.Kind != run.FailureTimeout {
		t.Fatalf("deadline = %#v, want timeout failure", err)
	}
}

func TestFailureKindForHTTPStatus(t *testing.T) {
	cases := []struct {
		status int
		want   run.FailureKind
	}{
		{http.StatusUnauthorized, run.FailureInvalidCredentials},
		{http.StatusForbidden, run.FailureInvalidCredentials},
		{http.StatusRequestTimeout, run.FailureTimeout},
		{http.StatusTooManyRequests, run.FailureRateLimited},
		{http.StatusBadRequest, run.FailureProviderRejected},
		{http.StatusServiceUnavailable, run.FailureProviderUnavailable},
	}
	for _, test := range cases {
		if got := failureKindForHTTPStatus(test.status); got != test.want {
			t.Errorf("status %d = %d, want %d", test.status, got, test.want)
		}
	}
}
