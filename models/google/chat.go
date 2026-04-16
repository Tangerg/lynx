package google

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"time"

	"google.golang.org/genai"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
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

func (r *requestHelper) buildConfig(opts *chat.Options) (*genai.GenerateContentConfig, error) {
	mergedOpts, err := chat.MergeOptions(r.defaultOptions, opts)
	if err != nil {
		return nil, err
	}

	cfg := getOptionsParams[genai.GenerateContentConfig](mergedOpts)

	if mergedOpts.Temperature != nil {
		v := float32(*mergedOpts.Temperature)
		cfg.Temperature = &v
	}
	if mergedOpts.TopP != nil {
		v := float32(*mergedOpts.TopP)
		cfg.TopP = &v
	}
	if mergedOpts.TopK != nil {
		v := float32(*mergedOpts.TopK)
		cfg.TopK = &v
	}
	if mergedOpts.MaxTokens != nil {
		cfg.MaxOutputTokens = int32(*mergedOpts.MaxTokens)
	}
	if len(mergedOpts.Stop) > 0 {
		cfg.StopSequences = mergedOpts.Stop
	}

	cfg.Tools, err = r.buildToolParams(mergedOpts.Tools)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (r *requestHelper) buildSystemInstruction(msgs []chat.Message) *genai.Content {
	systemMsg := chat.MergeSystemMessages(msgs)
	if systemMsg == nil || systemMsg.Text == "" {
		return nil
	}
	return genai.NewContentFromText(systemMsg.Text, "")
}

func (r *requestHelper) buildUserContent(msg *chat.UserMessage) *genai.Content {
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

func (r *requestHelper) buildAssistantContent(msg *chat.AssistantMessage) *genai.Content {
	parts := make([]*genai.Part, 0, 1+len(msg.ToolCalls))

	if msg.Text != "" {
		parts = append(parts, genai.NewPartFromText(msg.Text))
	}

	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if tc.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Arguments), &args)
		}
		parts = append(parts, genai.NewPartFromFunctionCall(tc.Name, args))
	}

	return genai.NewContentFromParts(parts, genai.RoleModel)
}

func (r *requestHelper) buildToolContent(msg *chat.ToolMessage) *genai.Content {
	parts := make([]*genai.Part, 0, len(msg.ToolReturns))

	for _, ret := range msg.ToolReturns {
		parts = append(parts, genai.NewPartFromFunctionResponse(ret.Name, map[string]any{
			"output": ret.Result,
		}))
	}

	return genai.NewContentFromParts(parts, genai.RoleUser)
}

func (r *requestHelper) buildContents(msgs []chat.Message) []*genai.Content {
	nonSystem := chat.FilterMessagesByMessageTypes(msgs, chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	contents := make([]*genai.Content, 0, len(nonSystem))
	for _, msg := range nonSystem {
		switch msg.Type() {
		case chat.MessageTypeUser:
			contents = append(contents, r.buildUserContent(msg.(*chat.UserMessage)))
		case chat.MessageTypeAssistant:
			contents = append(contents, r.buildAssistantContent(msg.(*chat.AssistantMessage)))
		case chat.MessageTypeTool:
			contents = append(contents, r.buildToolContent(msg.(*chat.ToolMessage)))
		}
	}
	return contents
}

func (r *requestHelper) buildApiChatRequest(req *chat.Request) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	cfg, err := r.buildConfig(req.Options)
	if err != nil {
		return "", nil, nil, err
	}

	cfg.SystemInstruction = r.buildSystemInstruction(req.Messages)
	contents := r.buildContents(req.Messages)
	modelName := r.defaultOptions.Model
	if req.Options != nil && req.Options.Model != "" {
		modelName = req.Options.Model
	}

	return modelName, contents, cfg, nil
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

	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			msgParams.Text += part.Text
		}
		if part.FunctionCall != nil {
			rawArgs, _ := json.Marshal(part.FunctionCall.Args)
			msgParams.ToolCalls = append(msgParams.ToolCalls, &chat.ToolCall{
				ID:        part.FunctionCall.ID,
				Name:      part.FunctionCall.Name,
				Arguments: string(rawArgs),
			})
		}
	}

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
		meta.Usage = &chat.Usage{
			PromptTokens:     int64(resp.UsageMetadata.PromptTokenCount),
			CompletionTokens: int64(resp.UsageMetadata.CandidatesTokenCount),
			OriginalUsage:    resp.UsageMetadata,
		}
	}

	meta.Set("original.response", resp)
	meta.Set("model_version", resp.ModelVersion)

	return meta
}

func (r *responseHelper) buildChatResponse(modelName string, resp *genai.GenerateContentResponse) (*chat.Response, error) {
	if len(resp.Candidates) == 0 {
		return nil, errors.New("google: no candidates in response")
	}

	results := make([]*chat.Result, 0, len(resp.Candidates))
	for _, candidate := range resp.Candidates {
		result, err := r.buildResult(candidate)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	meta := r.buildMeta(modelName, resp)
	return chat.NewResponse(results, meta)
}

type ChatModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *chat.Options
}

func (c *ChatModelConfig) validate() error {
	if c == nil {
		return errors.New("google: config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("google: api key is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("google: default options are required")
	}
	return nil
}

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *Api
	defaultOptions *chat.Options
	reqHelper      requestHelper
	respHelper     responseHelper
}

func NewChatModel(cfg *ChatModelConfig) (*ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{ApiKey: cfg.ApiKey})
	if err != nil {
		return nil, err
	}

	return &ChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		reqHelper:      requestHelper{cfg.DefaultOptions},
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

func (c *ChatModel) DefaultOptions() *chat.Options {
	return c.defaultOptions
}

func (c *ChatModel) Info() chat.ModelInfo {
	return chat.ModelInfo{
		Provider: Provider,
	}
}
