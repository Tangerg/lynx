package openai

import (
	"context"
	"iter"

	corechat "github.com/Tangerg/scope/core/chat"
)

const (
	// ResponsesRequestExtensionKey stores official Responses API parameters in
	// a Core request for fields without a provider-neutral equivalent.
	ResponsesRequestExtensionKey = "openai/responses_request"
	// ResponsesResponseExtensionKey preserves the complete official Responses
	// API response, including output item types Core does not normalize.
	ResponsesResponseExtensionKey = "openai/responses_response"
)

// ResponsesChat adapts OpenAI's ordered Responses API output to the minimal
// Core chat Model and Streamer capabilities.
type ResponsesChat struct {
	api      *api
	defaults corechat.Options
}

var (
	_ corechat.Model    = (*ResponsesChat)(nil)
	_ corechat.Streamer = (*ResponsesChat)(nil)
)

// NewResponsesChat constructs a Responses-API-backed Core chat adapter.
func NewResponsesChat(config ChatConfig) (*ResponsesChat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{APIKey: config.APIKey, BaseURL: config.BaseURL, HTTPClient: config.HTTPClient, Headers: config.Headers})
	if err != nil {
		return nil, err
	}
	return &ResponsesChat{api: api, defaults: config.DefaultOptions.Clone()}, nil
}

// Call performs one non-streaming Responses API request.
func (r *ResponsesChat) Call(ctx context.Context, req *corechat.Request) (*corechat.Response, error) {
	params, err := r.buildResponsesRequest(req)
	if err != nil {
		return nil, err
	}
	response, err := r.api.responseNew(ctx, params)
	if err != nil {
		return nil, err
	}
	return mapResponsesResponse(response)
}

// Stream performs one streaming Responses API request and yields ordered Core
// response deltas.
func (r *ResponsesChat) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		params, err := r.buildResponsesRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		stream, err := r.api.responseNewStream(ctx, params)
		if err != nil {
			yield(nil, err)
			return
		}
		defer stream.Close()

		state := newResponsesStreamState()
		for stream.Next() {
			response, include, mapErr := state.addEvent(stream.Current())
			if mapErr != nil {
				yield(nil, mapErr)
				return
			}
			if include && !yield(response, nil) {
				return
			}
		}
		if streamErr := stream.Err(); streamErr != nil {
			yield(nil, r.api.wrapError(streamErr))
		}
	}
}
