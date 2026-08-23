package llmadapter

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelcall"
	"github.com/Tangerg/lynx/app2/runtime/modelboundary"
)

type callOnlyModel struct{}

func (callOnlyModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return new(chat.Response), nil
}

type providerError struct {
	status int
	header http.Header
}

func (err *providerError) Error() string           { return "provider body must stay private" }
func (err *providerError) HTTPStatus() int         { return err.status }
func (err *providerError) HTTPHeader() http.Header { return err.header }

func TestClassifiedModelPreservesOptionalStreamingCapability(t *testing.T) {
	if _, ok := classifyModelFailures(callOnlyModel{}).(chat.Streamer); ok {
		t.Fatal("call-only model unexpectedly gained streaming capability")
	}
}

func TestClassifyModelErrorUsesProviderStatusAndRetryAfter(t *testing.T) {
	cause := &providerError{
		status: http.StatusTooManyRequests,
		header: http.Header{"Retry-After": []string{"12"}},
	}
	classified := classifyModelError(cause, time.Unix(1_000, 0))
	if !errors.Is(classified, cause) {
		t.Fatal("classification lost the provider error chain")
	}
	failure, ok := modelboundary.Decode(classified.Error())
	if !ok || failure.Kind() != modelcall.FailureRateLimited || failure.RetryAfterSeconds() != 12 {
		t.Fatalf("failure = %#v, %v", failure, ok)
	}
}

func TestClassifyModelErrorPreservesCancellationAndClassifiesDeadline(t *testing.T) {
	if got := classifyModelError(context.Canceled, time.Time{}); !errors.Is(got, context.Canceled) {
		t.Fatalf("cancellation = %v", got)
	}
	failure, ok := modelboundary.Decode(classifyModelError(context.DeadlineExceeded, time.Time{}).Error())
	if !ok || failure.Kind() != modelcall.FailureTimeout {
		t.Fatalf("deadline = %#v, %v", failure, ok)
	}
}

func TestFailureKindForHTTPStatus(t *testing.T) {
	tests := map[int]modelcall.FailureKind{
		http.StatusUnauthorized:       modelcall.FailureInvalidCredentials,
		http.StatusForbidden:          modelcall.FailureInvalidCredentials,
		http.StatusRequestTimeout:     modelcall.FailureTimeout,
		http.StatusTooManyRequests:    modelcall.FailureRateLimited,
		http.StatusBadRequest:         modelcall.FailureRejected,
		http.StatusServiceUnavailable: modelcall.FailureUnavailable,
	}
	for status, want := range tests {
		if got := failureKindForHTTPStatus(status); got != want {
			t.Errorf("status %d = %q, want %q", status, got, want)
		}
	}
}
