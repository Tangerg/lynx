package mistral

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"

	corechat "github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/sse"
)

const (
	RequestExtensionKey     = "mistral/request"
	responseExtensionKey    = "mistral/response"
	streamChunkExtensionKey = "mistral/chunk"
	mistralStreamDone       = "[DONE]"
	mistralStreamMaxBytes   = 16 << 20
)

// ReasoningEffort controls Mistral's native reasoning mode.
type ReasoningEffort string

const (
	ReasoningEffortHigh ReasoningEffort = "high"
	ReasoningEffortNone ReasoningEffort = "none"
)

func (r ReasoningEffort) Validate() error {
	switch r {
	case "", ReasoningEffortHigh, ReasoningEffortNone:
		return nil
	default:
		return fmt.Errorf("unsupported reasoning effort %q", r)
	}
}

// ChatRequestOptions exposes Mistral-specific Chat Completions parameters that
// have no provider-neutral Core equivalent. Store it under RequestExtensionKey.
type ChatRequestOptions struct {
	ReasoningEffort   ReasoningEffort   `json:"reasoning_effort,omitempty"`
	RandomSeed        *int64            `json:"random_seed,omitempty"`
	SafePrompt        *bool             `json:"safe_prompt,omitempty"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls,omitempty"`
	PromptCacheKey    string            `json:"prompt_cache_key,omitempty"`
	ToolChoice        json.RawMessage   `json:"tool_choice,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
	Guardrails        []json.RawMessage `json:"guardrails,omitempty"`
}

func (c ChatRequestOptions) Validate() error {
	if err := c.ReasoningEffort.Validate(); err != nil {
		return err
	}
	for name, raw := range map[string]json.RawMessage{
		"tool_choice": c.ToolChoice,
	} {
		if len(raw) > 0 && !json.Valid(raw) {
			return fmt.Errorf("%s contains invalid JSON", name)
		}
	}
	for index := range c.Guardrails {
		if !json.Valid(c.Guardrails[index]) {
			return fmt.Errorf("guardrails[%d] contains invalid JSON", index)
		}
	}
	return nil
}

// ChatConfig configures Mistral's native Chat Completions adapter.
type ChatConfig struct {
	APIKey         string
	DefaultOptions corechat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c ChatConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("mistral: APIKey is required")
	}
	if err := c.DefaultOptions.Validate(); err != nil {
		return fmt.Errorf("mistral: DefaultOptions: %w", err)
	}
	return nil
}

var (
	_ corechat.Model    = (*Chat)(nil)
	_ corechat.Streamer = (*Chat)(nil)
)

// Chat implements Mistral's native Chat Completions protocol, including
// structured thinking chunks and their multi-turn replay semantics.
type Chat struct {
	api      *api
	defaults corechat.Options
}

// NewChat constructs a Mistral Chat Completions adapter.
func NewChat(config ChatConfig) (*Chat, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	api, err := newAPI(apiConfig{
		APIKey:     config.APIKey,
		BaseURL:    cmp.Or(config.BaseURL, DefaultBaseURL),
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &Chat{api: api, defaults: config.DefaultOptions.Clone()}, nil
}

func (c *Chat) Call(ctx context.Context, request *corechat.Request) (*corechat.Response, error) {
	wireRequest, err := c.buildRequest(request, false)
	if err != nil {
		return nil, err
	}
	wireResponse, err := c.api.chatCompletion(ctx, wireRequest)
	if err != nil {
		return nil, err
	}
	return mapChatCompletion(wireResponse)
}

func (c *Chat) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	return func(yield func(*corechat.Response, error) bool) {
		wireRequest, err := c.buildRequest(request, true)
		if err != nil {
			yield(nil, err)
			return
		}
		body, err := c.api.chatCompletionStream(ctx, wireRequest)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		events := sse.NewReader(body)
		events.MaxLineBytes = mistralStreamMaxBytes
		events.MaxEventBytes = mistralStreamMaxBytes
		state := newChatStreamState()
		for event, eventErr := range events.Messages() {
			if eventErr != nil {
				yield(nil, fmt.Errorf("mistral: read chat stream: %w", eventErr))
				return
			}
			data := bytes.TrimSpace(event.Data)
			if bytes.Equal(data, []byte(mistralStreamDone)) {
				return
			}
			var chunk chatCompletionChunk
			if err := json.Unmarshal(data, &chunk); err != nil {
				yield(nil, fmt.Errorf("mistral: decode chat stream chunk: %w", err))
				return
			}
			response, err := state.mapChunk(chunk)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(response, nil) {
				return
			}
		}
	}
}
