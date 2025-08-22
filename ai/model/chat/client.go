package chat

import (
	"context"
	"errors"
	"iter"
	"slices"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/model"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

type CallHandler = model.CallHandler[*Request, *Response]
type StreamHandler = model.StreamHandler[*Request, *Response]
type CallHandlerFunc = model.CallHandlerFunc[*Request, *Response]
type StreamHandlerFunc = model.StreamHandlerFunc[*Request, *Response]
type CallMiddleware = model.CallMiddleware[*Request, *Response]
type StreamMiddleware = model.StreamMiddleware[*Request, *Response]

type MiddlewareManager = model.MiddlewareManager[*Request, *Response, *Request, *Response]

func NewMiddlewareManager() *MiddlewareManager {
	return &MiddlewareManager{}
}

const (
	OutputFormat = "lynx.ai.model.client.output_format"
)

type Client struct {
	defaultConfig *ClientConfig
}

func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	return &Client{
		defaultConfig: config.Clone(),
	}, nil
}

func (c *Client) Chat() *ClientConfig {
	return c.defaultConfig.Clone()
}

func (c *Client) ChatText(text string) *ClientConfig {
	msg := NewUserMessage(text)

	req, _ := NewRequest(
		[]Message{msg},
		c.defaultConfig.getOptions(),
	)

	return c.ChatRequest(req)
}

func (c *Client) ChatPromptTemplate(promptTemplate *PromptTemplate) *ClientConfig {
	return c.Chat().WithUserPromptTemplate(promptTemplate)
}

func (c *Client) ChatRequest(chatRequest *Request) *ClientConfig {
	cfg := c.defaultConfig.Clone()

	if chatRequest.Options() != nil {
		cfg.WithOptions(chatRequest.Options())
	}

	if len(chatRequest.Instructions()) > 0 {
		cfg.WithMessages(chatRequest.Instructions()...)
	}

	if len(chatRequest.Params()) > 0 {
		cfg.WithParams(chatRequest.Params())
	}

	return cfg
}

type ClientConfig struct {
	model                Model
	options              Options
	userPromptTemplate   *PromptTemplate
	systemPromptTemplate *PromptTemplate
	messages             []Message
	middlewareManager    *MiddlewareManager
	params               map[string]any
	tools                []Tool
	toolParams           map[string]any
}

func NewClientConfig(model Model) (*ClientConfig, error) {
	if model == nil {
		return nil, errors.New("model is required")
	}

	return &ClientConfig{
		model:             model,
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]Tool, 0),
		toolParams:        make(map[string]any),
	}, nil
}

func (c *ClientConfig) Call() *ClientCaller {
	return newClientCaller(c)
}

func (c *ClientConfig) Stream() *ClientStreamer {
	return newClientStreamer(c)
}

func (c *ClientConfig) WithOptions(options Options) *ClientConfig {
	if options != nil {
		c.options = options
	}
	return c
}

func (c *ClientConfig) WithUserPrompt(prompt string) *ClientConfig {
	if prompt != "" {
		c.userPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return c
}

func (c *ClientConfig) WithUserPromptTemplate(userPromptTemplate *PromptTemplate) *ClientConfig {
	if userPromptTemplate != nil {
		c.userPromptTemplate = userPromptTemplate
	}
	return c
}

func (c *ClientConfig) WithSystemPrompt(prompt string) *ClientConfig {
	if prompt != "" {
		c.systemPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return c
}

func (c *ClientConfig) WithSystemPromptTemplate(systemPromptTemplate *PromptTemplate) *ClientConfig {
	if systemPromptTemplate != nil {
		c.systemPromptTemplate = systemPromptTemplate
	}
	return c
}

func (c *ClientConfig) WithMessages(messages ...Message) *ClientConfig {
	if len(messages) > 0 {
		c.messages = messages
	}
	return c
}

func (c *ClientConfig) WithMiddlewares(middlewares ...any) *ClientConfig {
	if len(middlewares) > 0 {
		c.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return c
}

func (c *ClientConfig) WithMiddlewareManager(middlewareManager *MiddlewareManager) *ClientConfig {
	if middlewareManager != nil {
		c.middlewareManager = middlewareManager
	}
	return c
}

func (c *ClientConfig) WithParams(params map[string]any) *ClientConfig {
	if len(params) > 0 {
		c.params = params
	}
	return c
}

func (c *ClientConfig) WithTools(tools ...Tool) *ClientConfig {
	if len(tools) > 0 {
		c.tools = tools
	}
	return c
}

func (c *ClientConfig) WithToolParams(toolParams map[string]any) *ClientConfig {
	if len(toolParams) > 0 {
		c.toolParams = toolParams
	}
	return c
}

func (c *ClientConfig) Clone() *ClientConfig {
	cfg, _ := NewClientConfig(c.model)

	cfg.
		WithOptions(c.options).
		WithUserPromptTemplate(c.userPromptTemplate).
		WithSystemPromptTemplate(c.systemPromptTemplate).
		WithMessages(c.messages...).
		WithMiddlewareManager(c.middlewareManager).
		WithParams(c.params).
		WithTools(c.tools...).
		WithToolParams(c.toolParams)

	return cfg
}

func (c *ClientConfig) getOptions() Options {
	var opts Options

	if c.options != nil {
		opts = c.options.Clone()
	} else {
		opts = c.model.DefaultOptions().Clone()
	}

	toolOpts, ok := opts.(ToolOptions)
	if ok {
		toolOpts.AddTools(c.tools)
		toolOpts.AddToolParams(c.toolParams)
	}

	return opts
}

// getMessages processes and normalizes message sequences for AI conversation systems.
// This method ensures proper message structure and optimizes message sequences
// for AI model consumption.
//
// Processing Flow:
// 1. Initialize message list from template if empty
// 2. Process system messages (merge existing or render from template)
// 3. Add non-system messages (User, Assistant, Tool types)
// 4. Merge adjacent same-type messages for optimization
//
// Message Type Priority:
// - System messages: Always placed at the beginning
// - Other messages: Maintain original order (User, Assistant, Tool)
//
// Empty Message Handling:
// - If userPromptTemplate exists: render from template
// - Otherwise: create default greeting message "Hi!"
//
// Returns:
// - Processed message slice with merged adjacent same-type messages
// - Error if message rendering fails
func (c *ClientConfig) getMessages() ([]Message, error) {
	msgs := slices.Clone(c.messages)

	// Case 1: Handle empty message list
	// Initialize conversation with user prompt template or default greeting
	if len(msgs) == 0 {
		if c.userPromptTemplate != nil {
			userMsg, err := c.userPromptTemplate.RenderUserMessage()
			if err != nil {
				return nil, errors.Join(err, errors.New("failed to render user prompt template"))
			}
			msgs = append(msgs, userMsg)
		} else {
			// Use friendly greeting as fallback to ensure conversation can start
			defaultMsg := NewUserMessage("Hi!")
			msgs = append(msgs, defaultMsg)
		}
	}

	// Pre-allocate capacity for performance optimization
	// Reserve space for existing messages plus potential system message
	result := make([]Message, 0, len(msgs)+1)

	// Case 2: System message processing with priority-based selection
	// Strategy: Existing system messages take precedence over template-generated ones
	// Note: If neither existing system messages nor template exists, no system message is added
	sysMsg := MergeSystemMessages(msgs)
	if sysMsg != nil {
		// Priority 1: Use merged existing system messages
		result = append(result, sysMsg)
	} else if c.systemPromptTemplate != nil {
		// Priority 2: Generate system message from template when no existing ones found
		renderedSysMsg, err := c.systemPromptTemplate.RenderSystemMessage()
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to render system prompt template"))
		}
		result = append(result, renderedSysMsg)
	}

	// Case 3: Add non-system messages while preserving order
	// FilterMessages out system messages to prevent duplication since they're already processed above
	// Only include User, Assistant, and Tool messages in their original sequence
	filtered := FilterMessagesByMessageTypes(msgs, MessageTypeUser, MessageTypeAssistant, MessageTypeTool)
	result = append(result, filtered...)

	// Case 4: Final optimization - merge adjacent messages of the same type
	// This step combines consecutive messages of identical types to reduce redundancy
	// and optimize the message sequence for better AI model consumption
	// Example: [User1, User2, System, User3, Tool1, Tool2] → [MergedUser(1+2), System, User3, MergedTool(1+2)]
	final := MergeAdjacentSameTypeMessages(result)

	return final, nil
}

func (c *ClientConfig) getMiddlewareManager() *MiddlewareManager {
	if c.middlewareManager == nil {
		c.middlewareManager = NewMiddlewareManager()
	}
	return c.middlewareManager
}

func (c *ClientConfig) toRequest() (*Request, error) {
	msgs, err := c.getMessages()
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(msgs, c.getOptions())
	if err != nil {
		return nil, err
	}

	req.SetParams(c.params)

	return req, nil
}

type modelInvoker struct {
	chatModel Model
}

func newModelInvoker(chatModel Model) *modelInvoker {
	return &modelInvoker{
		chatModel: chatModel,
	}
}

func (i *modelInvoker) augmentLastUserMessageOutput(req *Request) {
	format, ok := req.Get(OutputFormat)
	if ok {
		req.augmentLastUserMessageText(cast.ToString(format))
	}
}

func (i *modelInvoker) Call(ctx context.Context, req *Request) (*Response, error) {
	i.augmentLastUserMessageOutput(req)
	return i.chatModel.Call(ctx, req)
}

func (i *modelInvoker) Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	i.augmentLastUserMessageOutput(req)
	return i.chatModel.Stream(ctx, req)
}

type ClientStreamer struct {
	config            *ClientConfig
	middlewareManager *MiddlewareManager
}

func newClientStreamer(config *ClientConfig) *ClientStreamer {
	return &ClientStreamer{
		config:            config,
		middlewareManager: config.getMiddlewareManager(),
	}
}

func (s *ClientStreamer) execute(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	handler := s.middlewareManager.MakeStreamHandler(newModelInvoker(s.config.model))
	return handler.Stream(ctx, req)
}

// TODO Due to the streaming nature, all data needs to be aggregated before conversion. Conversion functionality is temporarily not provided until a more elegant approach is found.
func (s *ClientStreamer) response(ctx context.Context, parser StructuredParser[any]) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		req, err := s.config.toRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		if parser != nil {
			req.Set(OutputFormat, parser.Instructions())
		}

		for resp, execErr := range s.execute(ctx, req) {
			if execErr != nil {
				yield(nil, execErr)
				return
			}

			if !yield(resp, nil) {
				return
			}
		}
	}
}

func (s *ClientStreamer) Response(ctx context.Context) iter.Seq2[*Response, error] {
	return s.response(ctx, nil)
}

func (s *ClientStreamer) Text(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for resp, err := range s.response(ctx, nil) {
			if err != nil {
				yield("", err)
				return
			}

			text := resp.Result().Output().Text()

			if !yield(text, nil) {
				return
			}
		}
	}
}

type ClientCaller struct {
	config            *ClientConfig
	middlewareManager *MiddlewareManager
}

func newClientCaller(config *ClientConfig) *ClientCaller {
	return &ClientCaller{
		config:            config,
		middlewareManager: config.getMiddlewareManager(),
	}
}

func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
	handler := c.middlewareManager.MakeCallHandler(newModelInvoker(c.config.model))
	return handler.Call(ctx, req)
}

func (c *ClientCaller) response(ctx context.Context, parser StructuredParser[any]) (*Response, error) {
	req, err := c.config.toRequest()
	if err != nil {
		return nil, err
	}

	if parser != nil {
		req.Set(OutputFormat, parser.Instructions())
	}

	return c.call(ctx, req)
}

func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	return c.response(ctx, nil)
}

func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.response(ctx, nil)
	if err != nil {
		return "", nil, err
	}

	text := resp.Result().Output().Text()
	return text, resp, nil
}

func (c *ClientCaller) List(ctx context.Context, listParser ...StructuredParser[[]string]) ([]string, *Response, error) {
	parser := pkgSlices.FirstOr(listParser, nil)
	if parser == nil {
		parser = NewListParser()
	}

	resp, err := c.response(ctx, ParserAsAny(parser))
	if err != nil {
		return nil, nil, err
	}

	data, err := parser.Parse(resp.Result().Output().Text())
	return data, resp, err
}

func (c *ClientCaller) Map(ctx context.Context, mapParser ...StructuredParser[map[string]any]) (map[string]any, *Response, error) {
	parser := pkgSlices.FirstOr(mapParser, nil)
	if parser == nil {
		parser = NewMapParser()
	}

	resp, err := c.response(ctx, ParserAsAny(parser))
	if err != nil {
		return nil, nil, err
	}

	data, err := parser.Parse(resp.Result().Output().Text())
	return data, resp, err
}

func (c *ClientCaller) Any(ctx context.Context, anyParser StructuredParser[any]) (any, *Response, error) {
	resp, err := c.response(ctx, anyParser)
	if err != nil {
		return nil, nil, err
	}

	data, err := anyParser.Parse(resp.Result().Output().Text())
	return data, resp, err
}
