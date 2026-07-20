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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	openaisdk "github.com/openai/openai-go/v3"
	"google.golang.org/genai"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
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
	var classified *execution.Failure
	if errors.As(err, &classified) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &execution.Failure{Kind: execution.FailureTimeout, Err: err}
	}
	if status, header, ok := providerHTTPError(err); ok {
		return &execution.Failure{
			Kind:       failureKindForHTTPStatus(status),
			RetryAfter: retryAfter(header, time.Now()),
			Err:        err,
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		kind := execution.FailureProviderUnavailable
		if netErr.Timeout() {
			kind = execution.FailureTimeout
		}
		return &execution.Failure{Kind: kind, Err: err}
	}
	return err
}

func providerHTTPError(err error) (int, http.Header, bool) {
	var openAIError *openaisdk.Error
	if errors.As(err, &openAIError) {
		return openAIError.StatusCode, responseHeader(openAIError.Response), true
	}
	var anthropicError *anthropicsdk.Error
	if errors.As(err, &anthropicError) {
		return anthropicError.StatusCode, responseHeader(anthropicError.Response), true
	}
	var googleError *genai.APIError
	if errors.As(err, &googleError) {
		return googleError.Code, nil, true
	}
	var azureError *azcore.ResponseError
	if errors.As(err, &azureError) {
		return azureError.StatusCode, responseHeader(azureError.RawResponse), true
	}
	return 0, nil, false
}

func responseHeader(response *http.Response) http.Header {
	if response == nil {
		return nil
	}
	return response.Header
}

func failureKindForHTTPStatus(status int) execution.FailureKind {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return execution.FailureInvalidCredentials
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		return execution.FailureTimeout
	case status == http.StatusTooManyRequests:
		return execution.FailureRateLimited
	case status >= http.StatusInternalServerError:
		return execution.FailureProviderUnavailable
	case status >= http.StatusBadRequest:
		return execution.FailureProviderRejected
	default:
		return execution.FailureInternal
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
