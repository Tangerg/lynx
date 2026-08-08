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

func (m failureModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	response, err := m.model.Call(ctx, request)
	return response, classifyModelError(err)
}

type failureStreamingModel struct {
	failureModel
	streamer chat.Streamer
}

func (m failureStreamingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	sequence := m.streamer.Stream(ctx, request)
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
	var classified *run.Failure
	if errors.As(err, &classified) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &run.Failure{Kind: run.FailureTimeout, Err: err}
	}
	if status, header, ok := providerHTTPError(err); ok {
		return &run.Failure{
			Kind:       failureKindForHTTPStatus(status),
			RetryAfter: retryAfter(header, time.Now()),
			Err:        err,
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		kind := run.FailureProviderUnavailable
		if netErr.Timeout() {
			kind = run.FailureTimeout
		}
		return &run.Failure{Kind: kind, Err: err}
	}
	return err
}

func providerHTTPError(err error) (int, http.Header, bool) {
	var responseError interface {
		HTTPStatus() int
		HTTPHeader() http.Header
	}
	if !errors.As(err, &responseError) {
		return 0, nil, false
	}
	return responseError.HTTPStatus(), responseError.HTTPHeader(), true
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
