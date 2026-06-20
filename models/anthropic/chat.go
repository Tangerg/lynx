package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/catalog"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/pkg/mime"
)

// defaultMaxTokens is sent when the caller leaves Options.MaxTokens
// unset. Anthropic's Messages API requires max_tokens.
const defaultMaxTokens int64 = 4096

type requestHelper struct {
	defaultOptions *chat.Options
}

func (r *requestHelper) buildToolParams(tools []chat.Tool) ([]anthropicsdk.ToolUnionParam, error) {
	toolParams := make([]anthropicsdk.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()

		var schema map[string]any
		if err := json.Unmarshal([]byte(def.InputSchema), &schema); err != nil {
			return nil, err
		}
		delete(schema, "type") // auto-set to "object" by ToolInputSchemaParam

		toolParam := anthropicsdk.ToolParam{
			Name:        def.Name,
			Description: param.NewOpt(def.Description),
			InputSchema: anthropicsdk.ToolInputSchemaParam{
				ExtraFields: schema,
			},
		}

		toolParams = append(toolParams, anthropicsdk.ToolUnionParam{OfTool: &toolParam})
	}

	return toolParams, nil
}

func (r *requestHelper) buildSystem(msgs []chat.Message) []anthropicsdk.TextBlockParam {
	systemMsg := chat.MergeSystemMessages(msgs)
	if systemMsg == nil || systemMsg.Text == "" {
		return nil
	}
	return []anthropicsdk.TextBlockParam{{Text: systemMsg.Text}}
}

func (r *requestHelper) buildBaseParams(opts *chat.Options) *anthropicsdk.MessageNewParams {
	params := options.GetParams[anthropicsdk.MessageNewParams](opts, OptionsKey)

	params.Model = opts.Model

	if opts.MaxTokens != nil {
		params.MaxTokens = *opts.MaxTokens
	} else if params.MaxTokens == 0 {
		params.MaxTokens = defaultMaxTokens
	}

	if opts.Temperature != nil {
		params.Temperature = param.NewOpt(*opts.Temperature)
	}
	if opts.TopP != nil {
		params.TopP = param.NewOpt(*opts.TopP)
	}
	if opts.TopK != nil {
		params.TopK = param.NewOpt(*opts.TopK)
	}
	if len(opts.Stop) > 0 {
		params.StopSequences = opts.Stop
	}

	return params
}

func (r *requestHelper) buildParams(opts *chat.Options, tools []chat.Tool) (*anthropicsdk.MessageNewParams, error) {
	mergedOpts, err := chat.MergeOptions(r.defaultOptions, opts)
	if err != nil {
		return nil, err
	}

	params := r.buildBaseParams(mergedOpts)

	// User-pre-staged tools (from Options.Extra, typically carrying a
	// cache_control breakpoint on the last entry) keep their position;
	// lynx-derived tools land after. Anthropic's cache_control is a
	// cumulative "everything before this point" marker, so appending
	// preserves the caller's caching intent.
	derivedTools, err := r.buildToolParams(tools)
	if err != nil {
		return nil, err
	}
	params.Tools = append(params.Tools, derivedTools...)

	return params, nil
}

func (r *requestHelper) buildUserMsg(msg *chat.UserMessage) anthropicsdk.MessageParam {
	if !msg.HasMedia() {
		return anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(msg.Text))
	}

	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, 1+len(msg.Media))
	if msg.Text != "" {
		blocks = append(blocks, anthropicsdk.NewTextBlock(msg.Text))
	}

	for _, md := range msg.Media {
		data, err := md.DataAsString()
		if err != nil {
			continue
		}

		mt := md.MimeType
		if mime.IsImage(mt) {
			blocks = append(blocks, anthropicsdk.NewImageBlockBase64(mt.TypeAndSubType(), data))
		} else if mt.FullType() == "application/pdf" {
			blocks = append(blocks, anthropicsdk.NewDocumentBlock(anthropicsdk.Base64PDFSourceParam{
				Data: data,
			}))
		} else {
			// treat other types as plain text documents
			blocks = append(blocks, anthropicsdk.NewDocumentBlock(anthropicsdk.PlainTextSourceParam{
				Data: data,
			}))
		}
	}

	return anthropicsdk.NewUserMessage(blocks...)
}

func (r *requestHelper) buildAssistantMsg(msg *chat.AssistantMessage) anthropicsdk.MessageParam {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(msg.Parts)+1)

	// Anthropic's content_block ordering is preserved verbatim — text /
	// reasoning / tool_use blocks are mapped 1:1 from lynx Parts in
	// emission order. Redacted thinking (a message-level metadata key)
	// is prepended when present.
	if data := redactedReasoning(msg); data != "" {
		blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(data))
	}

	for _, p := range msg.Parts {
		switch tp := p.(type) {
		case *chat.TextPart:
			if tp.Text != "" {
				blocks = append(blocks, anthropicsdk.NewTextBlock(tp.Text))
			}
		case *chat.ReasoningPart:
			// Anthropic requires a signature on every thinking block on
			// replay. Drop reasoning parts without signatures — they
			// usually mean the message came from a source that did not
			// preserve the continuation token.
			sig := string(tp.Signature)
			if sig == "" {
				continue
			}
			blocks = append(blocks, anthropicsdk.NewThinkingBlock(sig, tp.Text))
		case *chat.ToolCallPart:
			blocks = append(blocks, anthropicsdk.NewToolUseBlock(tp.ID, json.RawMessage(tp.Arguments), tp.Name))
		}
	}

	return anthropicsdk.NewAssistantMessage(blocks...)
}

func (r *requestHelper) buildToolMsg(msg *chat.ToolMessage) []anthropicsdk.MessageParam {
	// Anthropic expects all tool results in a single user message
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(msg.ToolReturns))

	for _, ret := range msg.ToolReturns {
		blocks = append(blocks, anthropicsdk.NewToolResultBlock(ret.ID, ret.Result, false))
	}

	if len(blocks) == 0 {
		return nil
	}
	return []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(blocks...)}
}

func (r *requestHelper) buildMsg(msg chat.Message) anthropicsdk.MessageParam {
	if msg.Type() == chat.MessageTypeUser {
		return r.buildUserMsg(msg.(*chat.UserMessage))
	}
	return r.buildAssistantMsg(msg.(*chat.AssistantMessage))
}

func (r *requestHelper) buildMsgs(msgs []chat.Message) []anthropicsdk.MessageParam {
	// Filter out system messages (handled separately)
	nonSystem := chat.FilterMessagesByMessageTypes(msgs, chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	result := make([]anthropicsdk.MessageParam, 0, len(nonSystem))
	for _, msg := range nonSystem {
		if msg.Type() == chat.MessageTypeTool {
			result = append(result, r.buildToolMsg(msg.(*chat.ToolMessage))...)
		} else {
			result = append(result, r.buildMsg(msg))
		}
	}
	return result
}

func (r *requestHelper) buildAPIChatRequest(req *chat.Request) (*anthropicsdk.MessageNewParams, error) {
	params, err := r.buildParams(req.Options, req.Tools)
	if err != nil {
		return nil, err
	}

	// Append lynx-derived blocks AFTER any pre-staged blocks the caller
	// shipped via Options.Extra. Anthropic prompt caching uses
	// cache_control as a cumulative breakpoint ("everything before this
	// point is cached"), so pre-staged System / Messages blocks must
	// keep their leading position for the caching intent to survive.
	params.System = append(params.System, r.buildSystem(req.Messages)...)
	params.Messages = append(params.Messages, r.buildMsgs(req.Messages)...)

	r.applyPromptCaching(params)

	return params, nil
}

// applyPromptCaching stamps default ephemeral cache_control breakpoints on
// the stable prefix of an assembled request so multi-round / multi-turn
// agentic work hits Anthropic's prompt cache (a cache read bills at ~10% of
// a fresh input token). Two breakpoints, the documented multi-turn pattern:
//
//   - the tail of the tools+system prefix — byte-identical on every call in
//     a session, so it is written once and read thereafter;
//   - the last block of the conversation — a rolling breakpoint that turns
//     each round's history into the next round's cached prefix (the cascade
//     that pays off most inside long tool loops).
//
// It is skipped in two cases, both deliberate:
//
//   - No tools. A toolless request is a one-shot utility call — compaction,
//     titling, extraction via askDirect — whose large transcript is summarized
//     once and never replayed. Caching it would only levy Anthropic's +25%
//     cache-WRITE surcharge on a prefix that is never read back. Tool presence
//     cleanly separates the looping main turn from those one-shots.
//   - The caller already staged a cache_control of their own (via
//     Options.Extra). Caching is then theirs to own; adding more risks
//     blowing Anthropic's four-breakpoint ceiling.
func (r *requestHelper) applyPromptCaching(params *anthropicsdk.MessageNewParams) {
	if len(params.Tools) == 0 || hasCacheControl(params) {
		return
	}

	if cc := params.Tools[len(params.Tools)-1].GetCacheControl(); cc != nil {
		*cc = anthropicsdk.NewCacheControlEphemeralParam()
	}

	if n := len(params.Messages); n > 0 {
		content := params.Messages[n-1].Content
		if m := len(content); m > 0 {
			if cc := content[m-1].GetCacheControl(); cc != nil {
				*cc = anthropicsdk.NewCacheControlEphemeralParam()
			}
		}
	}
}

// hasCacheControl reports whether any block in the assembled request already
// carries a cache_control breakpoint — meaning a caller staged caching via
// Options.Extra and applyPromptCaching must not second-guess it. lynx-derived
// blocks never carry one until applyPromptCaching runs, so any hit here is
// the caller's.
func hasCacheControl(params *anthropicsdk.MessageNewParams) bool {
	for i := range params.System {
		if !param.IsOmitted(params.System[i].CacheControl) {
			return true
		}
	}
	for i := range params.Tools {
		if cc := params.Tools[i].GetCacheControl(); cc != nil && !param.IsOmitted(*cc) {
			return true
		}
	}
	for i := range params.Messages {
		for j := range params.Messages[i].Content {
			if cc := params.Messages[i].Content[j].GetCacheControl(); cc != nil && !param.IsOmitted(*cc) {
				return true
			}
		}
	}
	return false
}

type responseHelper struct{}

func (r *responseHelper) mapFinishReason(stopReason anthropicsdk.StopReason) chat.FinishReason {
	switch stopReason {
	case anthropicsdk.StopReasonEndTurn, anthropicsdk.StopReasonStopSequence:
		return chat.FinishReasonStop
	case anthropicsdk.StopReasonMaxTokens:
		return chat.FinishReasonLength
	case anthropicsdk.StopReasonToolUse:
		return chat.FinishReasonToolCalls
	case anthropicsdk.StopReasonRefusal:
		return chat.FinishReasonContentFilter
	default:
		return chat.FinishReasonOther
	}
}

// buildAssistantMsg maps Anthropic content blocks 1:1 into lynx Parts,
// preserving original block ordering. Each thinking block carries its
// own signature on the ReasoningPart. Redacted thinking data lives in
// message-level Metadata under MetaRedactedReasoning (it has no
// counterpart Part in v1).
func (r *responseHelper) buildAssistantMsg(resp *anthropicsdk.Message) *chat.AssistantMessage {
	parts := make([]chat.OutputPart, 0, len(resp.Content))
	metadata := make(map[string]any)
	var redactedData string

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			parts = append(parts, &chat.TextPart{Text: block.Text})
		case "tool_use":
			rawInput, _ := json.Marshal(block.Input)
			parts = append(parts, &chat.ToolCallPart{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(rawInput),
			})
		case "thinking":
			parts = append(parts, &chat.ReasoningPart{
				Text:      block.Thinking,
				Signature: []byte(block.Signature),
			})
		case "redacted_thinking":
			redactedData = block.Data
		}
	}

	if redactedData != "" {
		metadata[MetaRedactedReasoning] = redactedData
	}

	return chat.NewAssistantMessage(chat.MessageParams{
		Parts:    parts,
		Metadata: metadata,
	})
}

func (r *responseHelper) buildResult(resp *anthropicsdk.Message) (*chat.Result, error) {
	msg := r.buildAssistantMsg(resp)
	return chat.NewResult(msg, &chat.ResultMetadata{
		FinishReason: r.mapFinishReason(resp.StopReason),
	})
}

func (r *responseHelper) buildMeta(resp *anthropicsdk.Message) *chat.ResponseMetadata {
	usage := &chat.Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		OriginalUsage:    resp.Usage,
	}
	// Surface Anthropic's prompt-cache breakdown when ephemeral caching
	// is in use. The SDK returns 0 when the field is absent from the
	// response payload, so a 0 value is indistinguishable from "the
	// provider did not surface this dimension"; any non-zero count
	// is treated as an explicit signal worth surfacing. Both fields are
	// subsets of InputTokens (= PromptTokens above).
	if v := resp.Usage.CacheReadInputTokens; v > 0 {
		usage.CacheReadInputTokens = &v
	}
	if v := resp.Usage.CacheCreationInputTokens; v > 0 {
		usage.CacheWriteInputTokens = &v
	}

	meta := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: time.Now().Unix(),
		Usage:   usage,
	}
	if resp.StopSequence != "" {
		meta.Set("stop_sequence", resp.StopSequence)
	}

	return meta
}

func (r *responseHelper) buildChatResponse(resp *anthropicsdk.Message) (*chat.Response, error) {
	result, err := r.buildResult(resp)
	if err != nil {
		return nil, err
	}

	meta := r.buildMeta(resp)

	return chat.NewResponse(result, meta)
}

// chunkAccumulator is the anthropic counterpart of openai-go's
// [openai.ChatCompletionAccumulator]: each [chunkAccumulator.AddChunk]
// consumes one stream event and returns a [*chat.Response] carrying
// ONLY that event's delta — Text fragment, tool-arg fragment,
// reasoning fragment, stop_reason, or usage update. The upstream
// [chat.ResponseAccumulator] stitches the deltas back together.
//
// We do not reuse the SDK's [anthropicsdk.Message].Accumulate because
// it is stateful (a content_block_delta event requires a matching
// prior content_block_start) — per-event reuse would error.
// chunkAccumulator carries only the minimum cross-event state:
//
//   - msgID / msgModel: lifted from message_start and tagged on every
//     subsequent yield so the response metadata stays stable;
//   - blockToToolID: maps Anthropic's content_block_index → the
//     ToolCallPart.ID assigned at start time, so subsequent
//     input_json_delta events can attach to the right tool call.
type chunkAccumulator struct {
	helper        *responseHelper
	msgID         string
	msgModel      string
	blockToToolID map[int64]string
}

// newChunkAccumulator returns an empty accumulator. Call
// [chunkAccumulator.AddChunk] for each event in order.
func (r *responseHelper) newChunkAccumulator() *chunkAccumulator {
	return &chunkAccumulator{helper: r, blockToToolID: map[int64]string{}}
}

// AddChunk converts one stream event into a per-event delta
// [*chat.Response] whose AssistantMessage.Parts holds just the new
// part deltas. Returns (nil, false) for events that produced no
// observable content (content_block_stop / message_stop).
func (a *chunkAccumulator) AddChunk(event anthropicsdk.MessageStreamEventUnion) (*chat.Response, bool) {
	var parts []chat.OutputPart
	resultMeta := &chat.ResultMetadata{}
	var metaUsage *chat.Usage
	hasContent := false

	switch e := event.AsAny().(type) {
	case anthropicsdk.MessageStartEvent:
		a.msgID = e.Message.ID
		a.msgModel = string(e.Message.Model)
		if e.Message.Usage.InputTokens > 0 || e.Message.Usage.OutputTokens > 0 {
			metaUsage = &chat.Usage{
				PromptTokens:     e.Message.Usage.InputTokens,
				CompletionTokens: e.Message.Usage.OutputTokens,
				OriginalUsage:    e.Message.Usage,
			}
			if v := e.Message.Usage.CacheReadInputTokens; v > 0 {
				metaUsage.CacheReadInputTokens = &v
			}
			if v := e.Message.Usage.CacheCreationInputTokens; v > 0 {
				metaUsage.CacheWriteInputTokens = &v
			}
			hasContent = true
		}

	case anthropicsdk.ContentBlockStartEvent:
		if block, ok := e.ContentBlock.AsAny().(anthropicsdk.ToolUseBlock); ok {
			a.blockToToolID[e.Index] = block.ID
			parts = []chat.OutputPart{&chat.ToolCallPart{
				ID:   block.ID,
				Name: block.Name,
			}}
			hasContent = true
		}
		// text / thinking / redacted_thinking blocks contribute no
		// delta at start time — content arrives via subsequent
		// content_block_delta events.

	case anthropicsdk.ContentBlockDeltaEvent:
		switch delta := e.Delta.AsAny().(type) {
		case anthropicsdk.TextDelta:
			if delta.Text != "" {
				parts = []chat.OutputPart{&chat.TextPart{Text: delta.Text}}
				hasContent = true
			}
		case anthropicsdk.InputJSONDelta:
			if delta.PartialJSON == "" {
				break
			}
			id, ok := a.blockToToolID[e.Index]
			if !ok {
				break
			}
			parts = []chat.OutputPart{&chat.ToolCallPart{
				ID:        id,
				Arguments: delta.PartialJSON,
			}}
			hasContent = true
		case anthropicsdk.ThinkingDelta:
			if delta.Thinking != "" {
				parts = []chat.OutputPart{&chat.ReasoningPart{Text: delta.Thinking}}
				hasContent = true
			}
		case anthropicsdk.SignatureDelta:
			if delta.Signature != "" {
				parts = []chat.OutputPart{&chat.ReasoningPart{Signature: []byte(delta.Signature)}}
				hasContent = true
			}
		}

	case anthropicsdk.MessageDeltaEvent:
		if e.Delta.StopReason != "" {
			resultMeta.FinishReason = a.helper.mapFinishReason(e.Delta.StopReason)
			hasContent = true
		}
		if e.Usage.OutputTokens > 0 {
			metaUsage = &chat.Usage{
				CompletionTokens: e.Usage.OutputTokens,
				OriginalUsage:    e.Usage,
			}
			hasContent = true
		}
		if e.Delta.StopSequence != "" {
			resultMeta.Set("stop_sequence", e.Delta.StopSequence)
		}

	case anthropicsdk.ContentBlockStopEvent, anthropicsdk.MessageStopEvent:
		// no observable content
	}

	if !hasContent {
		return nil, false
	}

	assistantMsg := chat.NewAssistantMessage(chat.MessageParams{Parts: parts})
	result, err := chat.NewResult(assistantMsg, resultMeta)
	if err != nil {
		return nil, false
	}
	meta := &chat.ResponseMetadata{
		ID:      a.msgID,
		Model:   a.msgModel,
		Created: time.Now().Unix(),
		Usage:   metaUsage,
	}
	resp, err := chat.NewResponse(result, meta)
	if err != nil {
		return nil, false
	}
	return resp, true
}

type ChatModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *chat.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [chat.ModelMetadata] returned by [ChatModel.Metadata].
	// Facades over this package (zhipu, minimax, moonshot, openrouter,
	// xiaomi, ...) pass their own Provider here. Zero Provider falls
	// back to the package default [Provider].
	Metadata *chat.ModelMetadata
}

func (c ChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("anthropic: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("anthropic: DefaultOptions is required")
	}
	return nil
}

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *API
	defaultOptions *chat.Options
	reqHelper      requestHelper
	respHelper     responseHelper
	metadata       chat.ModelMetadata
}

func NewChatModel(cfg ChatModelConfig) (*ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	info := catalog.Resolve(Provider, cfg.DefaultOptions, cfg.Metadata)
	return &ChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		reqHelper: requestHelper{
			cfg.DefaultOptions,
		},
		metadata: info,
	}, nil
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	apiReq, err := c.reqHelper.buildAPIChatRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := c.api.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return c.respHelper.buildChatResponse(apiResp)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		apiReq, err := c.reqHelper.buildAPIChatRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		apiStream := c.api.ChatCompletionStream(ctx, apiReq)
		defer apiStream.Close()

		// Per-stream chunk accumulator — mirrors OpenAI's per-chunk
		// fresh [openai.ChatCompletionAccumulator] approach. Each
		// AddChunk consumes one event and emits ONLY that event's
		// delta. The upstream [chat.ResponseAccumulator] stitches.
		acc := c.respHelper.newChunkAccumulator()

		for apiStream.Next() {
			resp, ok := acc.AddChunk(apiStream.Current())
			if !ok {
				continue
			}
			if !yield(resp, nil) {
				return
			}
		}

		if err = apiStream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return *c.defaultOptions
}

func (c *ChatModel) Metadata() chat.ModelMetadata {
	return c.metadata
}
