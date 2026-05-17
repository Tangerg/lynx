package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"

	ollamaapi "github.com/ollama/ollama/api"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/options"
)

type NativeChatModelConfig struct {
	DefaultOptions *chat.Options
	BaseURL        string
	HTTPClient     *http.Client
}

func (c *NativeChatModelConfig) validate() error {
	if c == nil {
		return errors.New("ollama: config must not be nil")
	}
	if c.DefaultOptions == nil {
		return errors.New("ollama: DefaultOptions is required")
	}
	return nil
}

var _ chat.Model = (*NativeChatModel)(nil)

// NativeChatModel wraps Ollama's native /api/chat endpoint. The native path
// gives access to Ollama-specific features (keep_alive, format=json,
// thinking on supported models); for an OpenAI-compatible flow point
// the openai provider at http://127.0.0.1:11434/v1 instead.
//
// Lynx's typed sampling knobs (Temperature, TopP, ...) route onto
// Ollama's flat "options" map. Top-level features not in our typed
// surface (KeepAlive, Format, Think) reach the wire through the
// Extra-threaded ollamaapi.ChatRequest.
type NativeChatModel struct {
	api            *Api
	defaultOptions *chat.Options
}

func NewNativeChatModel(cfg *NativeChatModelConfig) (*NativeChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		BaseURL:    cfg.BaseURL,
		HTTPClient: cfg.HTTPClient,
	})
	if err != nil {
		return nil, err
	}

	return &NativeChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
	}, nil
}

func (c *NativeChatModel) buildApiRequest(req *chat.Request, stream bool) (*ollamaapi.ChatRequest, error) {
	mergedOpts, err := chat.MergeOptions(c.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	apiReq := options.GetParams[ollamaapi.ChatRequest](mergedOpts, OptionsKey)
	apiReq.Model = mergedOpts.Model
	apiReq.Messages = c.buildMessages(req.Messages)
	apiReq.Stream = &stream

	tools, err := c.buildTools(req.Tools)
	if err != nil {
		return nil, err
	}
	apiReq.Tools = tools

	if apiReq.Options == nil {
		apiReq.Options = map[string]any{}
	}
	// Ollama merges sampling knobs into the flat "options" map. Only
	// write keys the caller actually set so we don't clobber values
	// the Extra-threaded request already placed there.
	if mergedOpts.Temperature != nil {
		apiReq.Options["temperature"] = float32(*mergedOpts.Temperature)
	}
	if mergedOpts.TopP != nil {
		apiReq.Options["top_p"] = float32(*mergedOpts.TopP)
	}
	if mergedOpts.TopK != nil {
		apiReq.Options["top_k"] = int(*mergedOpts.TopK)
	}
	if mergedOpts.MaxTokens != nil {
		apiReq.Options["num_predict"] = int(*mergedOpts.MaxTokens)
	}
	if mergedOpts.FrequencyPenalty != nil {
		apiReq.Options["frequency_penalty"] = float32(*mergedOpts.FrequencyPenalty)
	}
	if mergedOpts.PresencePenalty != nil {
		apiReq.Options["presence_penalty"] = float32(*mergedOpts.PresencePenalty)
	}
	if len(mergedOpts.Stop) > 0 {
		apiReq.Options["stop"] = mergedOpts.Stop
	}

	return apiReq, nil
}

func (c *NativeChatModel) buildTools(tools []chat.Tool) (ollamaapi.Tools, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make(ollamaapi.Tools, 0, len(tools))
	for _, t := range tools {
		def := t.Definition()
		var schema ollamaapi.ToolFunctionParameters
		if def.InputSchema != "" {
			if err := json.Unmarshal([]byte(def.InputSchema), &schema); err != nil {
				return nil, fmt.Errorf("ollama: tool %q has invalid input schema: %w", def.Name, err)
			}
		}
		out = append(out, ollamaapi.Tool{
			Type: "function",
			Function: ollamaapi.ToolFunction{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  schema,
			},
		})
	}
	return out, nil
}

func (c *NativeChatModel) buildMessages(msgs []chat.Message) []ollamaapi.Message {
	out := make([]ollamaapi.Message, 0, len(msgs))
	for _, msg := range msgs {
		switch m := msg.(type) {
		case *chat.SystemMessage:
			out = append(out, ollamaapi.Message{Role: "system", Content: m.Text})
		case *chat.UserMessage:
			images := make([]ollamaapi.ImageData, 0)
			for _, md := range m.Media {
				bytes, err := md.DataAsBytes()
				if err != nil {
					continue
				}
				images = append(images, bytes)
			}
			out = append(out, ollamaapi.Message{Role: "user", Content: m.Text, Images: images})
		case *chat.AssistantMessage:
			am := ollamaapi.Message{Role: "assistant", Content: m.Text, Thinking: m.Reasoning}
			for _, tc := range m.ToolCalls {
				call := ollamaapi.ToolCall{
					ID: tc.ID,
					Function: ollamaapi.ToolCallFunction{
						Name:      tc.Name,
						Arguments: parseToolArgs(tc.Arguments),
					},
				}
				am.ToolCalls = append(am.ToolCalls, call)
			}
			out = append(out, am)
		case *chat.ToolMessage:
			for _, ret := range m.ToolReturns {
				out = append(out, ollamaapi.Message{
					Role:       "tool",
					Content:    ret.Result,
					ToolName:   ret.Name,
					ToolCallID: ret.ID,
				})
			}
		}
	}
	return out
}

// parseToolArgs turns our string-shaped Arguments into Ollama's
// ordered-map shape. Failed parses yield an empty arguments object so
// the request stays well-formed.
func parseToolArgs(args string) ollamaapi.ToolCallFunctionArguments {
	out := ollamaapi.NewToolCallFunctionArguments()
	if args == "" {
		return out
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return out
	}
	for k, v := range raw {
		out.Set(k, v)
	}
	return out
}

func (c *NativeChatModel) buildResponse(apiResp ollamaapi.ChatResponse, apiReq *ollamaapi.ChatRequest) (*chat.Response, error) {
	msgParams := chat.MessageParams{
		Text:      apiResp.Message.Content,
		Reasoning: apiResp.Message.Thinking,
		Metadata:  make(map[string]any),
	}
	for _, tc := range apiResp.Message.ToolCalls {
		argsBytes, _ := json.Marshal(tc.Function.Arguments.ToMap())
		msgParams.ToolCalls = append(msgParams.ToolCalls, &chat.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: string(argsBytes),
		})
	}
	assistantMsg := chat.NewAssistantMessage(msgParams)

	resultMeta := &chat.ResultMetadata{
		FinishReason: mapDoneReason(apiResp.DoneReason),
	}

	result, err := chat.NewResult(assistantMsg, resultMeta)
	if err != nil {
		return nil, err
	}

	usage := &chat.Usage{
		PromptTokens:     int64(apiResp.PromptEvalCount),
		CompletionTokens: int64(apiResp.EvalCount),
		OriginalUsage:    apiResp.Metrics,
	}

	meta := &chat.ResponseMetadata{
		Model:   apiResp.Model,
		Created: apiResp.CreatedAt.Unix(),
		Usage:   usage,
	}
	meta.Set("total_duration_ms", apiResp.TotalDuration.Milliseconds())
	meta.Set("load_duration_ms", apiResp.LoadDuration.Milliseconds())
	meta.Set("prompt_eval_duration_ms", apiResp.PromptEvalDuration.Milliseconds())
	meta.Set("eval_duration_ms", apiResp.EvalDuration.Milliseconds())

	return chat.NewResponse(result, meta)
}

func mapDoneReason(reason string) chat.FinishReason {
	switch reason {
	case "stop":
		return chat.FinishReasonStop
	case "length":
		return chat.FinishReasonLength
	case "":
		return chat.FinishReasonNull
	default:
		return chat.FinishReasonOther
	}
}

func (c *NativeChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	apiReq, err := c.buildApiRequest(req, false)
	if err != nil {
		return nil, err
	}

	// Non-stream mode still goes through the callback, but Ollama
	// fires it exactly once with the complete reply when Stream=false.
	var final ollamaapi.ChatResponse
	err = c.api.Chat(ctx, apiReq, func(resp ollamaapi.ChatResponse) error {
		final = resp
		return nil
	})
	if err != nil {
		return nil, err
	}

	return c.buildResponse(final, apiReq)
}

func (c *NativeChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		apiReq, err := c.buildApiRequest(req, true)
		if err != nil {
			yield(nil, err)
			return
		}

		// Bridge Ollama's callback-style stream into an iter.Seq2 by
		// having the callback yield each chunk. The callback returns an
		// error to abort the stream when the consumer stops pulling.
		var consumerStopped bool
		err = c.api.Chat(ctx, apiReq, func(chunk ollamaapi.ChatResponse) error {
			if consumerStopped {
				return context.Canceled
			}
			out, buildErr := c.buildResponse(chunk, apiReq)
			if buildErr != nil {
				return buildErr
			}
			if !yield(out, nil) {
				consumerStopped = true
				return context.Canceled
			}
			return nil
		})
		if err != nil && !consumerStopped {
			yield(nil, err)
		}
	}
}

func (c *NativeChatModel) DefaultOptions() chat.Options {
	return *c.defaultOptions
}

func (c *NativeChatModel) Metadata() chat.ModelMetadata {
	return chat.ModelMetadata{
		Provider: Provider,
	}
}
