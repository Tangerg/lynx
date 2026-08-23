package llmadapter

import (
	"context"
	"errors"
	"iter"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelcall"
	"github.com/Tangerg/lynx/app2/runtime/modelboundary"
)

type failureModel struct {
	model chat.Model
	now   func() time.Time
}

func classifyModelFailures(model chat.Model) chat.Model {
	classified := failureModel{model: model, now: time.Now}
	streamer, ok := model.(chat.Streamer)
	if !ok {
		return classified
	}
	return failureStreamingModel{failureModel: classified, streamer: streamer}
}

func (model failureModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	response, err := model.model.Call(ctx, request)
	return response, classifyModelError(err, model.now())
}

type failureStreamingModel struct {
	failureModel
	streamer chat.Streamer
}

func (model failureStreamingModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	sequence := model.streamer.Stream(ctx, request)
	if sequence == nil {
		return nil
	}
	return func(yield func(*chat.Response, error) bool) {
		for response, err := range sequence {
			if !yield(response, classifyModelError(err, model.now())) {
				return
			}
		}
	}
}

func classifyModelError(err error, now time.Time) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	kind := modelcall.FailureKind("")
	retryAfterSeconds := 0
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind = modelcall.FailureTimeout
	default:
		if status, header, ok := providerHTTPError(err); ok {
			kind = failureKindForHTTPStatus(status)
			retryAfterSeconds = retryAfter(header, now)
		} else {
			var networkError net.Error
			if errors.As(err, &networkError) {
				kind = modelcall.FailureUnavailable
				if networkError.Timeout() {
					kind = modelcall.FailureTimeout
				}
			}
		}
	}
	if !kind.Valid() {
		return err
	}
	if kind != modelcall.FailureRateLimited && kind != modelcall.FailureUnavailable {
		retryAfterSeconds = 0
	}
	failure, failureErr := modelcall.NewFailure(kind, retryAfterSeconds)
	if failureErr != nil {
		return err
	}
	return modelboundary.Carry(failure, err)
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

func failureKindForHTTPStatus(status int) modelcall.FailureKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return modelcall.FailureInvalidCredentials
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return modelcall.FailureTimeout
	case status == http.StatusTooManyRequests:
		return modelcall.FailureRateLimited
	case status >= http.StatusInternalServerError:
		return modelcall.FailureUnavailable
	case status >= http.StatusBadRequest:
		return modelcall.FailureRejected
	default:
		return ""
	}
}

func retryAfter(header http.Header, now time.Time) int {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		return int(seconds)
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return int(math.Ceil(when.Sub(now).Seconds()))
}
