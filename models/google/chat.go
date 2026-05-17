package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/options"
)

type requestHelper struct {
	defaultOptions *chat.Options
}

func (r *requestHelper) buildToolParams(tools []chat.Tool) ([]*genai.Tool, error) {
	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()

		var schema map[string]any
		if err := json.Unmarshal([]byte(def.InputSchema), &schema); err != nil {
			return nil, err
		}

		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:                 def.Name,
			Description:          def.Description,
			ParametersJsonSchema: schema,
		})
	}

	if len(declarations) == 0 {
		return nil, nil
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}, nil
}

func (r *requestHelper) buildParams(mergedOpts *chat.Options, tools []chat.Tool) (*genai.GenerateContentConfig, error) {
	cfg := options.GetParams[genai.GenerateContentConfig](mergedOpts, OptionsKey)

	if mergedOpts.Temperature != nil {
		cfg.Temperature = new(float32(*mergedOpts.Temperature))
	}
	if mergedOpts.TopP != nil {
		cfg.TopP = new(float32(*mergedOpts.TopP))
	}
	if mergedOpts.TopK != nil {
		cfg.TopK = new(float32(*mergedOpts.TopK))
	}
	if mergedOpts.MaxTokens != nil {
		cfg.MaxOutputTokens = int32(*mergedOpts.MaxTokens)
	}
	if len(mergedOpts.Stop) > 0 {
		cfg.StopSequences = mergedOpts.Stop
	}

	toolParams, err := r.buildToolParams(tools)
	if err != nil {
		return nil, err
	}
	cfg.Tools = toolParams

	return cfg, nil
}

func (r *requestHelper) buildSystem(msgs []chat.Message) *genai.Content {
	systemMsg := chat.MergeSystemMessages(msgs)
	if systemMsg == nil || systemMsg.Text == "" {
		return nil
	}
	return genai.NewContentFromText(systemMsg.Text, "")
}

func (r *requestHelper) buildUserMsg(msg *chat.UserMessage) *genai.Content {
	parts := make([]*genai.Part, 0, 1+len(msg.Media))

	if msg.Text != "" {
		parts = append(parts, genai.NewPartFromText(msg.Text))
	}

	for _, md := range msg.Media {
		data, err := md.DataAsBytes()
		if err != nil {
			continue
		}
		parts = append(parts, genai.NewPartFromBytes(data, md.MimeType.TypeAndSubType()))
	}

	return genai.NewContentFromParts(parts, genai.RoleUser)
}

func (r *requestHelper) buildAssistantMsg(msg *chat.AssistantMessage) (*genai.Content, error) {
	parts := make([]*genai.Part, 0, 1+len(msg.ToolCalls))

	if msg.Text != "" {
		parts = append(parts, genai.NewPartFromText(msg.Text))
	}

	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				return nil, fmt.Errorf("google: tool call %q arguments are not valid JSON: %w", tc.Name, err)
			}
		}
		parts = append(parts, genai.NewPartFromFunctionCall(tc.Name, args))
	}

	return genai.NewContentFromParts(parts, genai.RoleModel), nil
}

func (r *requestHelper) buildToolMsg(msg *chat.ToolMessage) *genai.Content {
	parts := make([]*genai.Part, 0, len(msg.ToolReturns))

	for _, ret := range msg.ToolReturns {
		parts = append(parts, genai.NewPartFromFunctionResponse(ret.Name, toolReturnAsObject(ret.Result)))
	}

	return genai.NewContentFromParts(parts, genai.RoleUser)
}

// toolReturnAsObject converts our string-shaped ToolReturn.Result into the
// JSON object Gemini's FunctionResponse expects. We try to parse the
// payload as a JSON object first so structured results round-trip
// untouched; non-JSON or non-object results fall back to {"output": str}
// so the LLM still sees the body.
func toolReturnAsObject(result string) map[string]any {
	if result == "" {
		return map[string]any{"output": ""}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(result), &obj); err == nil {
		return obj
	}
	return map[string]any{"output": result}
}

func (r *requestHelper) buildMsgs(msgs []chat.Message) ([]*genai.Content, error) {
	nonSystem := chat.FilterMessagesByMessageTypes(msgs, chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	contents := make([]*genai.Content, 0, len(nonSystem))
	for _, msg := range nonSystem {
		switch msg.Type() {
		case chat.MessageTypeUser:
			contents = append(contents, r.buildUserMsg(msg.(*chat.UserMessage)))
		case chat.MessageTypeAssistant:
			content, err := r.buildAssistantMsg(msg.(*chat.AssistantMessage))
			if err != nil {
				return nil, err
			}
			contents = append(contents, content)
		case chat.MessageTypeTool:
			contents = append(contents, r.buildToolMsg(msg.(*chat.ToolMessage)))
		}
	}
	return contents, nil
}

func (r *requestHelper) buildApiChatRequest(req *chat.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	mergedOpts, err := chat.MergeOptions(r.defaultOptions, req.Options)
	if err != nil {
		return "", nil, nil, err
	}

	cfg, err := r.buildParams(mergedOpts, req.Tools)
	if err != nil {
		return "", nil, nil, err
	}

	cfg.SystemInstruction = r.buildSystem(req.Messages)
	contents, err := r.buildMsgs(req.Messages)
	if err != nil {
		return "", nil, nil, err
	}

	return mergedOpts.Model, contents, cfg, nil
}

type responseHelper struct{}

func (r *responseHelper) mapFinishReason(reason genai.FinishReason) chat.FinishReason {
	switch reason {
	case genai.FinishReasonStop:
		return chat.FinishReasonStop
	case genai.FinishReasonMaxTokens:
		return chat.FinishReasonLength
	case genai.FinishReasonSafety, genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent, genai.FinishReasonSPII,
		genai.FinishReasonImageSafety, genai.FinishReasonImageProhibitedContent:
		return chat.FinishReasonContentFilter
	case genai.FinishReasonMalformedFunctionCall, genai.FinishReasonUnexpectedToolCall:
		return chat.FinishReasonToolCalls
	default:
		return chat.FinishReasonOther
	}
}

func (r *responseHelper) buildAssistantMsg(candidate *genai.Candidate) *chat.AssistantMessage {
	msgParams := chat.MessageParams{
		Metadata: make(map[string]any),
	}

	if candidate.Content == nil {
		return chat.NewAssistantMessage(msgParams)
	}

	var textBuf, reasoningBuf strings.Builder
	for _, part := range candidate.Content.Parts {
		switch {
		case part.FunctionCall != nil:
			rawArgs, _ := json.Marshal(part.FunctionCall.Args)
			msgParams.ToolCalls = append(msgParams.ToolCalls, &chat.ToolCall{
				ID:        part.FunctionCall.ID,
				Name:      part.FunctionCall.Name,
				Arguments: string(rawArgs),
			})
		case part.Thought:
			// Gemini 2.5 thinking: parts flagged Thought=true carry
			// chain-of-thought text. Route into Reasoning rather than
			// Text so they survive accumulation without polluting the
			// visible reply.
			reasoningBuf.WriteString(part.Text)
		case part.Text != "":
			textBuf.WriteString(part.Text)
		}
	}

	msgParams.Text = textBuf.String()
	msgParams.Reasoning = reasoningBuf.String()

	return chat.NewAssistantMessage(msgParams)
}

func (r *responseHelper) buildResult(candidate *genai.Candidate) (*chat.Result, error) {
	assistantMsg := r.buildAssistantMsg(candidate)
	meta := &chat.ResultMetadata{
		FinishReason: r.mapFinishReason(candidate.FinishReason),
	}
	meta.Set("index", candidate.Index)
	return chat.NewResult(assistantMsg, meta)
}

func (r *responseHelper) buildMeta(modelName string, resp *genai.GenerateContentResponse) *chat.ResponseMetadata {
	meta := &chat.ResponseMetadata{
		ID:      resp.ResponseID,
		Model:   modelName,
		Created: time.Now().Unix(),
	}

	if resp.UsageMetadata != nil {
		usage := &chat.Usage{
			PromptTokens:     int64(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
			OriginalUsage:    resp.UsageMetadata,
		}
		// Surface Gemini's prompt-cache hit count (subset of PromptTokens).
		// Treat any non-zero count as the provider explicitly reporting
		// the dimension; absent / zero stays nil to preserve "unknown".
		if v := int64(resp.UsageMetadata.CachedContentTokenCount); v > 0 {
			usage.CacheReadInputTokens = &v
		}
		// Surface Gemini 2.5 thinking-token breakdown (subset of
		// CompletionTokens), mapping onto the same shape the OpenAI o-series
		// and Claude extended thinking use.
		if v := int64(resp.UsageMetadata.ThoughtsTokenCount); v > 0 {
			usage.ReasoningTokens = &v
		}
		meta.Usage = usage

		if v := resp.UsageMetadata.ToolUsePromptTokenCount; v > 0 {
			meta.Set("tool_use_prompt_tokens", int64(v))
		}
	}

	meta.Set("model_version", resp.ModelVersion)

	return meta
}

func (r *responseHelper) buildChatResponse(modelName string, resp *genai.GenerateContentResponse) (*chat.Response, error) {
	// Gemini returns Candidates[] sized by GenerationConfig.CandidateCount
	// (default 1). The chat surface is single-completion by design — see
	// chat.Response — so we take Candidates[0] and ignore extras. Callers
	// needing N>1 should drop down to the genai SDK directly.
	if len(resp.Candidates) == 0 {
		return nil, errors.New("google: no candidates in response")
	}

	result, err := r.buildResult(resp.Candidates[0])
	if err != nil {
		return nil, err
	}

	meta := r.buildMeta(modelName, resp)
	return chat.NewResponse(result, meta)
}

type ChatModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *chat.Options

	// Backend selects the genai backend. Zero value defaults to
	// [genai.BackendGeminiAPI] — the public Gemini API where ApiKey
	// is required. Set to [genai.BackendVertexAI] for GCP-hosted
	// inference; Project / Location become required and auth flows
	// through Application Default Credentials.
	Backend genai.Backend

	// Project is the GCP project id — required when Backend ==
	// [genai.BackendVertexAI].
	Project string

	// Location is the GCP region (e.g. "us-central1") — required
	// when Backend == [genai.BackendVertexAI].
	Location string

	// Metadata overrides the [chat.ModelMetadata] returned by [ChatModel.Metadata].
	// The vertexai facade passes its own Provider here. Zero Provider
	// falls back to the package default [Provider].
	Metadata *chat.ModelMetadata
}

func (c *ChatModelConfig) validate() error {
	if c == nil {
		return errors.New("google: config must not be nil")
	}
	if c.Backend != genai.BackendVertexAI && c.ApiKey == nil {
		return errors.New("google: ApiKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("google: DefaultOptions is required")
	}
	return nil
}

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *Api
	defaultOptions *chat.Options
	reqHelper      requestHelper
	respHelper     responseHelper
	metadata       chat.ModelMetadata
}

func NewChatModel(cfg *ChatModelConfig) (*ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:   cfg.ApiKey,
		Backend:  cfg.Backend,
		Project:  cfg.Project,
		Location: cfg.Location,
	})
	if err != nil {
		return nil, err
	}

	info := chat.ModelMetadata{Provider: Provider}
	if cfg.Metadata != nil {
		info = *cfg.Metadata
	}
	return &ChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		reqHelper:      requestHelper{cfg.DefaultOptions},
		metadata:           info,
	}, nil
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	modelName, contents, cfg, err := c.reqHelper.buildApiChatRequest(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.api.ChatCompletion(ctx, modelName, contents, cfg)
	if err != nil {
		return nil, err
	}

	return c.respHelper.buildChatResponse(modelName, resp)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		modelName, contents, cfg, err := c.reqHelper.buildApiChatRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		for resp, err := range c.api.ChatCompletionStream(ctx, modelName, contents, cfg) {
			if err != nil {
				yield(nil, err)
				return
			}

			chatResp, err := c.respHelper.buildChatResponse(modelName, resp)
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(chatResp, nil) {
				return
			}
		}
	}
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return *c.defaultOptions
}

func (c *ChatModel) Metadata() chat.ModelMetadata {
	return c.metadata
}
