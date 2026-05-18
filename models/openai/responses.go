package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/options"
)

// ResponsesChatModel adapts the OpenAI Responses API (/v1/responses) to
// the [chat.Model] surface, alongside the legacy [ChatModel] (which
// targets /v1/chat/completions).
//
// Unlike Chat Completions — whose wire format splits text and tool_calls
// into separate fields, forcing the adapter to reconstruct a "reasoning
// → text → tool_calls" Parts order — the Responses API returns an
// ordered `output[]` of items (message / function_call / reasoning).
// This adapter maps each output_item 1:1 to a [chat.OutputPart], so
// interleaved "text → tool_call → text → tool_call → text" turns
// survive the round trip the same way they do on the Anthropic and
// Bedrock adapters.
//
// v1 scope: stateless mode only (no `previous_response_id`); three
// output items handled (message text → [chat.TextPart], reasoning →
// [chat.ReasoningPart], function_call → [chat.ToolCallPart]). Built-in
// tools (web_search / file_search / code_interpreter / mcp_call /
// image_generation / shell / computer_call) are surfaced via the raw
// SDK but produce no Parts in this version — extending [chat.OutputPart]
// is a follow-up epic.
type ResponsesChatModel struct {
	api            *Api
	defaultOptions *chat.Options
	reqHelper      responsesRequestHelper
	respHelper     responsesResponseHelper
	metadata       chat.ModelMetadata
}

var _ chat.Model = (*ResponsesChatModel)(nil)

// NewResponsesChatModel wires up a Responses-API-backed chat model.
// Config is identical to [NewChatModel] — same ApiKey, DefaultOptions,
// RequestOptions, optional Metadata override — so callers can switch
// surfaces without re-plumbing.
func NewResponsesChatModel(cfg *ChatModelConfig) (*ResponsesChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:         cfg.ApiKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	info := chat.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}

	return &ResponsesChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		reqHelper:      responsesRequestHelper{defaultOptions: cfg.DefaultOptions},
		metadata:       info,
	}, nil
}

func (c *ResponsesChatModel) DefaultOptions() chat.Options { return *c.defaultOptions }
func (c *ResponsesChatModel) Metadata() chat.ModelMetadata { return c.metadata }

func (c *ResponsesChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	apiReq, err := c.reqHelper.build(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := c.api.ResponseNew(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return c.respHelper.build(apiResp)
}

func (c *ResponsesChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		apiReq, err := c.reqHelper.build(req)
		if err != nil {
			yield(nil, err)
			return
		}

		stream, err := c.api.ResponseNewStream(ctx, apiReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer stream.Close()

		acc := newResponsesChunkAccumulator()
		for stream.Next() {
			event := stream.Current()
			resp, ok := acc.addEvent(event)
			if !ok {
				continue
			}
			if !yield(resp, nil) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// --------------------------------------------------------------------
// Request side: lynx Request → ResponseNewParams.
// --------------------------------------------------------------------

type responsesRequestHelper struct {
	defaultOptions *chat.Options
}

func (r *responsesRequestHelper) build(req *chat.Request) (*responses.ResponseNewParams, error) {
	mergedOpts, err := chat.MergeOptions(r.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	params := options.GetParams[responses.ResponseNewParams](mergedOpts, OptionsKey)
	params.Model = shared.ResponsesModel(mergedOpts.Model)

	if mergedOpts.MaxTokens != nil {
		params.MaxOutputTokens = openai.Int(*mergedOpts.MaxTokens)
	}
	if mergedOpts.Temperature != nil {
		params.Temperature = openai.Float(*mergedOpts.Temperature)
	}
	if mergedOpts.TopP != nil {
		params.TopP = openai.Float(*mergedOpts.TopP)
	}

	// Ask the server to ship back reasoning's encrypted_content; lynx
	// stores it on ReasoningPart.Signature so reasoning items can
	// round-trip in the next turn even with store=false (default).
	if !slices.Contains(params.Include, responses.ResponseIncludableReasoningEncryptedContent) {
		params.Include = append(params.Include, responses.ResponseIncludableReasoningEncryptedContent)
	}

	items, err := r.buildInputItems(req.Messages)
	if err != nil {
		return nil, err
	}
	params.Input.OfInputItemList = items

	params.Tools, err = r.buildTools(req.Tools)
	if err != nil {
		return nil, err
	}

	return params, nil
}

func (r *responsesRequestHelper) buildTools(tools []chat.Tool) ([]responses.ToolUnionParam, error) {
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		def := t.Definition()
		var params map[string]any
		if err := json.Unmarshal([]byte(def.InputSchema), &params); err != nil {
			return nil, err
		}
		out = append(out, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        def.Name,
				Description: openai.String(def.Description),
				Parameters:  params,
				Strict:      openai.Bool(true),
			},
		})
	}
	return out, nil
}

func (r *responsesRequestHelper) buildInputItems(msgs []chat.Message) (responses.ResponseInputParam, error) {
	out := make(responses.ResponseInputParam, 0, len(msgs))
	for _, msg := range msgs {
		more, err := r.itemsFromMessage(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

func (r *responsesRequestHelper) itemsFromMessage(msg chat.Message) ([]responses.ResponseInputItemUnionParam, error) {
	switch m := msg.(type) {
	case *chat.SystemMessage:
		return []responses.ResponseInputItemUnionParam{easyMessage(responses.EasyInputMessageRoleSystem, m.Text)}, nil
	case *chat.UserMessage:
		return []responses.ResponseInputItemUnionParam{easyMessage(responses.EasyInputMessageRoleUser, m.Text)}, nil
	case *chat.AssistantMessage:
		return assistantItems(m), nil
	case *chat.ToolMessage:
		return toolResultItems(m), nil
	default:
		return nil, fmt.Errorf("openai responses: unsupported message type %T", msg)
	}
}

// easyMessage builds a role-tagged plain-text input item — sidesteps
// the required-ID + content-list ceremony that
// ResponseInputItemMessageParam carries.
func easyMessage(role responses.EasyInputMessageRole, text string) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{
			Role:    role,
			Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(text)},
		},
	}
}

// assistantItems projects AssistantMessage.Parts to ordered Responses
// API input items — one per Part:
//
//   - TextPart      → role=assistant EasyInputMessage (avoids the
//     OutputMessage ID requirement; preserves position in the stream).
//   - ReasoningPart → reasoning item, but only when Signature is set
//     (no signature = no round-trip token, so the item carries no
//     value to a follow-up turn).
//   - ToolCallPart  → function_call item (CallID is the correlation key
//     for function_call_output).
func assistantItems(msg *chat.AssistantMessage) []responses.ResponseInputItemUnionParam {
	out := make([]responses.ResponseInputItemUnionParam, 0, len(msg.Parts))

	for i, p := range msg.Parts {
		switch part := p.(type) {
		case *chat.TextPart:
			if part.Text == "" {
				continue
			}
			out = append(out, easyMessage(responses.EasyInputMessageRoleAssistant, part.Text))

		case *chat.ReasoningPart:
			if len(part.Signature) == 0 {
				continue
			}
			item := &responses.ResponseReasoningItemParam{
				ID:               fmt.Sprintf("rs_lynx_%d", i),
				EncryptedContent: openai.String(string(part.Signature)),
			}
			if part.Text != "" {
				item.Summary = []responses.ResponseReasoningItemSummaryParam{{Text: part.Text}}
			}
			out = append(out, responses.ResponseInputItemUnionParam{OfReasoning: item})

		case *chat.ToolCallPart:
			out = append(out, responses.ResponseInputItemUnionParam{
				OfFunctionCall: &responses.ResponseFunctionToolCallParam{
					CallID:    part.ID,
					Name:      part.Name,
					Arguments: part.Arguments,
				},
			})
		}
	}

	return out
}

func toolResultItems(msg *chat.ToolMessage) []responses.ResponseInputItemUnionParam {
	out := make([]responses.ResponseInputItemUnionParam, 0, len(msg.ToolReturns))
	for _, ret := range msg.ToolReturns {
		out = append(out, responses.ResponseInputItemUnionParam{
			OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
				CallID: ret.ID,
				Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
					OfString: openai.String(ret.Result),
				},
			},
		})
	}
	return out
}

// --------------------------------------------------------------------
// Response side: Response → chat.Response.
// --------------------------------------------------------------------

type responsesResponseHelper struct{}

func (r *responsesResponseHelper) build(resp *responses.Response) (*chat.Response, error) {
	if resp == nil {
		return nil, errors.New("openai responses: nil response")
	}

	parts, hasToolCalls := partsFromOutput(resp.Output)
	assistantMsg := chat.NewAssistantMessage(chat.MessageParams{Parts: parts})
	resultMeta := &chat.ResultMetadata{
		FinishReason: deriveFinishReason(resp, hasToolCalls),
	}
	respMeta := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   string(resp.Model),
		Created: int64(resp.CreatedAt),
		Usage:   usageFrom(resp.Usage),
	}

	result, err := chat.NewResult(assistantMsg, resultMeta)
	if err != nil {
		return nil, err
	}
	return chat.NewResponse(result, respMeta)
}

// partsFromOutput walks the ordered output[] and projects each
// supported item to a lynx OutputPart, preserving wire order. Returns
// (parts, hasFunctionCall) — the bool is used to derive FinishReason
// when the response itself doesn't tag it (the Responses API uses
// `status`, not a finish_reason field).
func partsFromOutput(out []responses.ResponseOutputItemUnion) ([]chat.OutputPart, bool) {
	parts := make([]chat.OutputPart, 0, len(out))
	hasToolCall := false

	for _, item := range out {
		switch item.Type {
		case "message":
			msg := item.AsMessage()
			for _, c := range msg.Content {
				if c.Type == "output_text" && c.Text != "" {
					parts = append(parts, &chat.TextPart{Text: c.Text})
				}
			}
		case "reasoning":
			r := item.AsReasoning()
			rp := &chat.ReasoningPart{Text: joinReasoning(r)}
			if r.EncryptedContent != "" {
				rp.Signature = []byte(r.EncryptedContent)
			}
			parts = append(parts, rp)
		case "function_call":
			fc := item.AsFunctionCall()
			id := fc.CallID
			if id == "" {
				id = fc.ID
			}
			parts = append(parts, &chat.ToolCallPart{
				ID:        id,
				Name:      fc.Name,
				Arguments: fc.Arguments,
			})
			hasToolCall = true
		}
		// Built-in tool output items (file_search_call, web_search_call,
		// image_generation_call, code_interpreter_call, mcp_*, shell_*,
		// computer_call, ...) are intentionally dropped in v1 — extending
		// chat.OutputPart with vendor-specific built-ins is a follow-up.
	}

	return parts, hasToolCall
}

// joinReasoning prefers full reasoning text (with encrypted content
// requested via Include) over the summary-only payload returned to
// non-reasoning callers.
func joinReasoning(r responses.ResponseReasoningItem) string {
	var b strings.Builder
	if len(r.Content) > 0 {
		for _, c := range r.Content {
			b.WriteString(c.Text)
		}
	} else {
		for _, s := range r.Summary {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

func deriveFinishReason(resp *responses.Response, hasToolCall bool) chat.FinishReason {
	if hasToolCall {
		return chat.FinishReasonToolCalls
	}
	if resp.Status == "incomplete" {
		switch resp.IncompleteDetails.Reason {
		case "max_output_tokens":
			return chat.FinishReasonLength
		case "content_filter":
			return chat.FinishReasonContentFilter
		}
	}
	return chat.FinishReasonStop
}

func usageFrom(u responses.ResponseUsage) *chat.Usage {
	out := &chat.Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		OriginalUsage:    u,
	}
	if rt := u.OutputTokensDetails.ReasoningTokens; rt > 0 {
		out.ReasoningTokens = &rt
	}
	if ct := u.InputTokensDetails.CachedTokens; ct > 0 {
		out.CacheReadInputTokens = &ct
	}
	return out
}

// --------------------------------------------------------------------
// Stream side: per-event delta Response.
// --------------------------------------------------------------------

// responsesChunkAccumulator turns one upstream Responses stream event
// into at most one delta [*chat.Response]. State across events:
//
//   - respID / respModel: captured on response.created so every delta
//     can ship the same response-level metadata.
//   - callIDByItemID: function_call_arguments.delta events reference
//     only the item_id; the matching function_call's call_id is what
//     lynx pairs with ToolMessage.ToolReturns. We learn the mapping
//     on response.output_item.added and look it up on each delta.
type responsesChunkAccumulator struct {
	respID         string
	respModel      string
	callIDByItemID map[string]string
}

func newResponsesChunkAccumulator() *responsesChunkAccumulator {
	return &responsesChunkAccumulator{callIDByItemID: map[string]string{}}
}

func (a *responsesChunkAccumulator) addEvent(event responses.ResponseStreamEventUnion) (*chat.Response, bool) {
	switch e := event.AsAny().(type) {
	case responses.ResponseCreatedEvent:
		a.respID = e.Response.ID
		a.respModel = string(e.Response.Model)
		return nil, false

	case responses.ResponseOutputItemAddedEvent:
		switch e.Item.Type {
		case "function_call":
			fc := e.Item.AsFunctionCall()
			a.callIDByItemID[fc.ID] = fc.CallID
			return a.deltaResponse([]chat.OutputPart{&chat.ToolCallPart{
				ID:   fc.CallID,
				Name: fc.Name,
			}}), true
		}
		return nil, false

	case responses.ResponseTextDeltaEvent:
		if e.Delta == "" {
			return nil, false
		}
		return a.deltaResponse([]chat.OutputPart{&chat.TextPart{Text: e.Delta}}), true

	case responses.ResponseFunctionCallArgumentsDeltaEvent:
		if e.Delta == "" {
			return nil, false
		}
		callID := a.callIDByItemID[e.ItemID]
		return a.deltaResponse([]chat.OutputPart{&chat.ToolCallPart{
			ID:        callID,
			Arguments: e.Delta,
		}}), true

	case responses.ResponseReasoningTextDeltaEvent:
		if e.Delta == "" {
			return nil, false
		}
		return a.deltaResponse([]chat.OutputPart{&chat.ReasoningPart{Text: e.Delta}}), true

	case responses.ResponseReasoningSummaryTextDeltaEvent:
		if e.Delta == "" {
			return nil, false
		}
		return a.deltaResponse([]chat.OutputPart{&chat.ReasoningPart{Text: e.Delta}}), true

	case responses.ResponseOutputItemDoneEvent:
		// Reasoning items carry their encrypted_content only on `done`;
		// surface it as a Signature-only ReasoningPart delta so the
		// upstream accumulator merges it into the in-flight reasoning.
		if e.Item.Type == "reasoning" {
			r := e.Item.AsReasoning()
			if r.EncryptedContent != "" {
				return a.deltaResponse([]chat.OutputPart{&chat.ReasoningPart{
					Signature: []byte(r.EncryptedContent),
				}}), true
			}
		}
		return nil, false

	case responses.ResponseCompletedEvent:
		// Final tick: ship usage + finish reason. No new Parts here —
		// they already arrived via the per-item deltas above.
		hasToolCall := slices.ContainsFunc(e.Response.Output, func(item responses.ResponseOutputItemUnion) bool {
			return item.Type == "function_call"
		})
		result := &chat.Result{
			Metadata: &chat.ResultMetadata{FinishReason: deriveFinishReason(&e.Response, hasToolCall)},
		}
		resp, _ := chat.NewResponse(result, a.responseMeta(usageFrom(e.Response.Usage)))
		return resp, true
	}

	return nil, false
}

func (a *responsesChunkAccumulator) deltaResponse(parts []chat.OutputPart) *chat.Response {
	result := &chat.Result{
		AssistantMessage: chat.NewAssistantMessage(parts),
		Metadata:         &chat.ResultMetadata{},
	}
	resp, _ := chat.NewResponse(result, a.responseMeta(nil))
	return resp
}

func (a *responsesChunkAccumulator) responseMeta(usage *chat.Usage) *chat.ResponseMetadata {
	return &chat.ResponseMetadata{
		ID:    a.respID,
		Model: a.respModel,
		Usage: usage,
	}
}

