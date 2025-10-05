package chat

import (
	"context"
	"errors"
	"iter"
	"maps"
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

// NewMiddlewareManager creates a new middleware manager for chat operations.
func NewMiddlewareManager() *MiddlewareManager {
	return &MiddlewareManager{}
}

// OutputFormat is the parameter key for specifying response format instructions.
const OutputFormat = "lynx.ai.model.client.output_format"

// Client provides a high-level interface for chat interactions
// with configurable defaults and fluent API support.
type Client struct {
	defaultConfig *ClientConfig
}

// NewClient creates a new chat client with the specified default configuration.
// Returns an error if config is nil.
func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		return nil, errors.New("client config is required")
	}

	return &Client{
		defaultConfig: config.Clone(),
	}, nil
}

// Chat returns a new configuration based on the client's defaults
// for creating customized chat interactions.
func (c *Client) Chat() *ClientConfig {
	return c.defaultConfig.Clone()
}

// ChatText creates a chat interaction with a simple text message
// using default options.
func (c *Client) ChatText(text string) *ClientConfig {
	msg := NewUserMessage(text)

	req := &Request{
		Messages: []Message{msg},
		Options:  c.defaultConfig.getOptions(),
	}

	return c.ChatRequest(req)
}

// ChatPromptTemplate creates a chat interaction using a prompt template
// for the user message.
func (c *Client) ChatPromptTemplate(promptTemplate *PromptTemplate) *ClientConfig {
	return c.Chat().WithUserPromptTemplate(promptTemplate)
}

// ChatRequest creates a chat interaction from an existing request,
// merging its configuration with client defaults.
func (c *Client) ChatRequest(chatRequest *Request) *ClientConfig {
	cfg := c.defaultConfig.Clone()

	cfg.WithOptions(chatRequest.Options).
		WithMessages(chatRequest.Messages...).
		WithParams(chatRequest.Params)

	return cfg
}

// ClientConfig provides fluent configuration for chat interactions
// including messages, options, templates, and middleware.
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

// NewClientConfig creates a new client configuration with the specified model.
// Returns an error if model is nil.
func NewClientConfig(model Model) (*ClientConfig, error) {
	if model == nil {
		return nil, errors.New("chat model is required")
	}

	return &ClientConfig{
		model:             model,
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]Tool, 0),
		toolParams:        make(map[string]any),
	}, nil
}

// Call returns a caller for synchronous chat operations.
func (c *ClientConfig) Call() *ClientCaller {
	return newClientCaller(c)
}

// Stream returns a streamer for streaming chat operations.
func (c *ClientConfig) Stream() *ClientStreamer {
	return newClientStreamer(c)
}

// WithOptions sets the model options for the chat interaction.
// Returns the config for method chaining.
func (c *ClientConfig) WithOptions(options Options) *ClientConfig {
	if options != nil {
		c.options = options
	}
	return c
}

// WithUserPrompt sets the user prompt from a string template.
// Returns the config for method chaining.
func (c *ClientConfig) WithUserPrompt(prompt string) *ClientConfig {
	if prompt != "" {
		c.userPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return c
}

// WithUserPromptTemplate sets the user prompt template.
// Returns the config for method chaining.
func (c *ClientConfig) WithUserPromptTemplate(userPromptTemplate *PromptTemplate) *ClientConfig {
	if userPromptTemplate != nil {
		c.userPromptTemplate = userPromptTemplate
	}
	return c
}

// WithSystemPrompt sets the system prompt from a string template.
// Returns the config for method chaining.
func (c *ClientConfig) WithSystemPrompt(prompt string) *ClientConfig {
	if prompt != "" {
		c.systemPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return c
}

// WithSystemPromptTemplate sets the system prompt template.
// Returns the config for method chaining.
func (c *ClientConfig) WithSystemPromptTemplate(systemPromptTemplate *PromptTemplate) *ClientConfig {
	if systemPromptTemplate != nil {
		c.systemPromptTemplate = systemPromptTemplate
	}
	return c
}

// WithMessages appends messages to the conversation.
// Returns the config for method chaining.
func (c *ClientConfig) WithMessages(messages ...Message) *ClientConfig {
	if len(messages) > 0 {
		c.messages = messages
	}
	return c
}

// WithMiddlewares sets the middleware chain for the chat interaction.
// Returns the config for method chaining.
func (c *ClientConfig) WithMiddlewares(middlewares ...any) *ClientConfig {
	if len(middlewares) > 0 {
		c.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return c
}

// WithMiddlewareManager sets the middleware manager.
// Returns the config for method chaining.
func (c *ClientConfig) WithMiddlewareManager(middlewareManager *MiddlewareManager) *ClientConfig {
	if middlewareManager != nil {
		c.middlewareManager = middlewareManager
	}
	return c
}

// WithParams sets additional parameters for the chat request.
// Returns the config for method chaining.
func (c *ClientConfig) WithParams(params map[string]any) *ClientConfig {
	if len(params) > 0 {
		c.params = params
	}
	return c
}

// WithTools adds tools available for the chat interaction.
// Returns the config for method chaining.
func (c *ClientConfig) WithTools(tools ...Tool) *ClientConfig {
	if len(tools) > 0 {
		c.tools = tools
	}
	return c
}

// WithToolParams sets parameters for tool execution.
// Returns the config for method chaining.
func (c *ClientConfig) WithToolParams(toolParams map[string]any) *ClientConfig {
	if len(toolParams) > 0 {
		c.toolParams = toolParams
	}
	return c
}

// Clone creates a deep copy of the configuration.
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

// getOptions returns the effective options for the chat interaction,
// merging config options with tool-specific settings.
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

// getMiddlewareManager returns the middleware manager, initializing if needed.
func (c *ClientConfig) getMiddlewareManager() *MiddlewareManager {
	if c.middlewareManager == nil {
		c.middlewareManager = NewMiddlewareManager()
	}
	return c.middlewareManager
}

// toRequest converts the configuration to a chat request.
func (c *ClientConfig) toRequest() (*Request, error) {
	msgs, err := c.getMessages()
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	req.Options = c.getOptions()
	req.Params = maps.Clone(c.params)

	return req, nil
}

// modelInvoker handles direct model invocation with output format augmentation.
type modelInvoker struct {
	chatModel Model
}

// newModelInvoker creates a new model invoker for the specified chat model.
func newModelInvoker(chatModel Model) *modelInvoker {
	return &modelInvoker{
		chatModel: chatModel,
	}
}

// augmentLastUserMessageOutput appends output format instructions to the last user message
// if OutputFormat parameter is present in the request.
func (i *modelInvoker) augmentLastUserMessageOutput(req *Request) {
	format, ok := req.Get(OutputFormat)
	if ok {
		req.augmentLastUserMessageText(cast.ToString(format))
	}
}

// Call invokes the model synchronously with output format augmentation.
func (i *modelInvoker) Call(ctx context.Context, req *Request) (*Response, error) {
	i.augmentLastUserMessageOutput(req)
	return i.chatModel.Call(ctx, req)
}

// Stream invokes the model in streaming mode with output format augmentation.
func (i *modelInvoker) Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	i.augmentLastUserMessageOutput(req)
	return i.chatModel.Stream(ctx, req)
}

// ClientStreamer handles streaming chat operations with middleware support.
type ClientStreamer struct {
	config            *ClientConfig
	middlewareManager *MiddlewareManager
}

// newClientStreamer creates a new client streamer with the specified configuration.
func newClientStreamer(config *ClientConfig) *ClientStreamer {
	return &ClientStreamer{
		config:            config,
		middlewareManager: config.getMiddlewareManager(),
	}
}

// execute runs the streaming chat operation through the middleware chain.
func (s *ClientStreamer) execute(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	handler := s.middlewareManager.MakeStreamHandler(newModelInvoker(s.config.model))
	return handler.Stream(ctx, req)
}

// response performs the streaming chat operation with optional structured parsing.
// Note: Structured parsing is not yet implemented for streaming due to data aggregation requirements.
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

// Response streams chat responses without structured parsing.
func (s *ClientStreamer) Response(ctx context.Context) iter.Seq2[*Response, error] {
	return s.response(ctx, nil)
}

// Text streams text content from assistant messages.
func (s *ClientStreamer) Text(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for resp, err := range s.response(ctx, nil) {
			if err != nil {
				yield("", err)
				return
			}

			if !yield(resp.Result().AssistantMessage.Text, nil) {
				return
			}
		}
	}
}

// ClientCaller handles synchronous chat operations with middleware support.
type ClientCaller struct {
	config            *ClientConfig
	middlewareManager *MiddlewareManager
}

// newClientCaller creates a new client caller with the specified configuration.
func newClientCaller(config *ClientConfig) *ClientCaller {
	return &ClientCaller{
		config:            config,
		middlewareManager: config.getMiddlewareManager(),
	}
}

// call executes the synchronous chat operation through the middleware chain.
func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
	handler := c.middlewareManager.MakeCallHandler(newModelInvoker(c.config.model))
	return handler.Call(ctx, req)
}

// response performs the chat operation with optional structured parsing.
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

// Response executes the chat and returns the raw response.
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	return c.response(ctx, nil)
}

// Text executes the chat and returns the assistant's text response.
func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.response(ctx, nil)
	if err != nil {
		return "", nil, err
	}

	return resp.Result().AssistantMessage.Text, resp, nil
}

// List executes the chat with list parsing and returns structured list data.
// Uses default list parser if none provided.
func (c *ClientCaller) List(ctx context.Context, listParser ...StructuredParser[[]string]) ([]string, *Response, error) {
	parser := pkgSlices.FirstOr(listParser, nil)
	if parser == nil {
		parser = NewListParser()
	}

	resp, err := c.response(ctx, ParserAsAny(parser))
	if err != nil {
		return nil, nil, err
	}

	data, err := parser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, err
}

// Map executes the chat with map parsing and returns structured map data.
// Uses default map parser if none provided.
func (c *ClientCaller) Map(ctx context.Context, mapParser ...StructuredParser[map[string]any]) (map[string]any, *Response, error) {
	parser := pkgSlices.FirstOr(mapParser, nil)
	if parser == nil {
		parser = NewMapParser()
	}

	resp, err := c.response(ctx, ParserAsAny(parser))
	if err != nil {
		return nil, nil, err
	}

	data, err := parser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, err
}

// Any executes the chat with custom structured parsing.
func (c *ClientCaller) Any(ctx context.Context, anyParser StructuredParser[any]) (any, *Response, error) {
	resp, err := c.response(ctx, anyParser)
	if err != nil {
		return nil, nil, err
	}

	data, err := anyParser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, err
}
