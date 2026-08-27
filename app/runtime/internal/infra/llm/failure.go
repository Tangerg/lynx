package llm

import (
	"context"
	"errors"
	"iter"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/core/chat"
)

// failureModel translates provider-specific errors at the infrastructure
// boundary. The rest of the runtime sees one typed execution failure taxonomy
// and never parses provider error strings.
type failureModel struct {
	model chat.Model
}

func classifyModelFailures(model chat.Model) chat.Model {
	classified := failureModel{model: model}
	streamer, ok := model.(chat.Streamer)
	if !ok {
		return classified
	}
	return failureStreamingModel{failureModel: classified, streamer: streamer}
}

func (f failureModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	response, err := f.model.Call(ctx, request)
	return response, classifyModelError(err)
}

type failureStreamingModel struct {
	failureModel
	streamer chat.Streamer
}

func (f failureStreamingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	sequence := f.streamer.Stream(ctx, request)
	if sequence == nil {
		return nil
	}
	return func(yield func(*chat.Response, error) bool) {
		for response, err := range sequence {
			if !yield(response, classifyModelError(err)) {
				return
			}
		}
	}
}

func classifyModelError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	if _, ok := errors.AsType[*run.FailureError](err); ok {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &run.FailureError{Kind: run.FailureTimeout, Err: err}
	}
	if status, header, ok := providerHTTPError(err); ok {
		return &run.FailureError{
			Kind:       failureKindForHTTPStatus(status),
			RetryAfter: retryAfter(header, time.Now()),
			Err:        err,
		}
	}
	if netErr, ok := errors.AsType[net.Error](err); ok {
		kind := run.FailureProviderUnavailable
		if netErr.Timeout() {
			kind = run.FailureTimeout
		}
		return &run.FailureError{Kind: kind, Err: err}
	}
	return err
}

func providerHTTPError(err error) (int, http.Header, bool) {
	type httpError interface {
		error
		HTTPStatus() int
		HTTPHeader() http.Header
	}
	matched, ok := errors.AsType[httpError](err)
	if !ok {
		return 0, nil, false
	}
	return matched.HTTPStatus(), matched.HTTPHeader(), true
}

func failureKindForHTTPStatus(status int) run.FailureKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return run.FailureInvalidCredentials
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return run.FailureTimeout
	case status == http.StatusTooManyRequests:
		return run.FailureRateLimited
	case status >= http.StatusInternalServerError:
		return run.FailureProviderUnavailable
	case status >= http.StatusBadRequest:
		return run.FailureProviderRejected
	default:
		return run.FailureInternal
	}
}

func retryAfter(header http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}
