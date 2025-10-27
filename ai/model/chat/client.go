package chat

import (
	"context"
	"errors"
	"iter"
	"maps"
	"slices"

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
	return model.NewMiddlewareManager[*Request, *Response, *Request, *Response]()
}

// ClientRequest provides fluent configuration for chat interactions
// including messages, options, templates, and middleware.
type ClientRequest struct {
	model                Model
	middlewareManager    *MiddlewareManager
	options              *Options
	userPromptTemplate   *PromptTemplate
	systemPromptTemplate *PromptTemplate
	messages             []Message
	params               map[string]any
	tools                []Tool
}

// NewClientRequest creates a new client request with the specified model.
// Returns an error if model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("chat model cannot be nil")
	}

	return &ClientRequest{
		model:             model,
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]Tool, 0),
	}, nil
}

// WithMiddlewares sets the middleware chain for the request, replacing any existing middlewares.
// Middlewares are executed in the order they are provided.
// Returns the request for method chaining.
func (r *ClientRequest) WithMiddlewares(middlewares ...any) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return r
}

// WithMiddlewareManager sets the middleware manager.
// Returns the request for method chaining.
func (r *ClientRequest) WithMiddlewareManager(middlewareManager *MiddlewareManager) *ClientRequest {
	if middlewareManager != nil {
		r.middlewareManager = middlewareManager
	}
	return r
}

// WithOptions sets the model options for the chat interaction.
// Returns the request for method chaining.
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithUserPrompt sets the user prompt from a string template.
// Returns the request for method chaining.
func (r *ClientRequest) WithUserPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.userPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return r
}

// WithUserPromptTemplate sets the user prompt template.
// Returns the request for method chaining.
func (r *ClientRequest) WithUserPromptTemplate(template *PromptTemplate) *ClientRequest {
	if template != nil {
		r.userPromptTemplate = template
	}
	return r
}

// WithSystemPrompt sets the system prompt from a string template.
// Returns the request for method chaining.
func (r *ClientRequest) WithSystemPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.systemPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return r
}

// WithSystemPromptTemplate sets the system prompt template.
// Returns the request for method chaining.
func (r *ClientRequest) WithSystemPromptTemplate(template *PromptTemplate) *ClientRequest {
	if template != nil {
		r.systemPromptTemplate = template
	}
	return r
}

// WithMessages appends messages to the conversation.
// Returns the request for method chaining.
func (r *ClientRequest) WithMessages(messages ...Message) *ClientRequest {
	if len(messages) > 0 {
		r.messages = messages
	}
	return r
}

// WithParams sets additional parameters for the chat request.
// Returns the request for method chaining.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = params
	}
	return r
}

// WithTools adds tools available for the chat interaction.
// Returns the request for method chaining.
func (r *ClientRequest) WithTools(tools ...Tool) *ClientRequest {
	if len(tools) > 0 {
		r.tools = tools
	}
	return r
}

// MiddlewareManager returns the middleware manager, initializing if needed.
func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

// Clone creates a deep copy of the client request.
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:                r.model,
		middlewareManager:    r.middlewareManager.Clone(),
		options:              r.options.Clone(),
		userPromptTemplate:   r.userPromptTemplate.Clone(),
		systemPromptTemplate: r.systemPromptTemplate.Clone(),
		messages:             slices.Clone(r.messages),
		params:               maps.Clone(r.params),
		tools:                slices.Clone(r.tools),
	}
}

// getOptions returns the effective options for the chat interaction,
// merging request options with tool-specific settings.
func (r *ClientRequest) getOptions() *Options {
	var opts *Options

	if r.options != nil {
		opts = r.options.Clone()
	} else {
		opts = r.model.DefaultOptions().Clone()
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
func (r *ClientRequest) getMessages() ([]Message, error) {
	msgs := slices.Clone(r.messages)

	// Case 1: Handle empty message list
	// Initialize conversation with user prompt template or default greeting
	if len(msgs) == 0 {
		if r.userPromptTemplate != nil {
			userMsg, err := r.userPromptTemplate.CreateUserMessage()
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, userMsg)
		} else {
			// Use friendly greeting as fallback to ensure conversation can start
			msgs = append(msgs, NewUserMessage("Hi!"))
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
	} else if r.systemPromptTemplate != nil {
		// Priority 2: Generate system message from template when no existing ones found
		renderedMsg, err := r.systemPromptTemplate.CreateSystemMessage()
		if err != nil {
			return nil, err
		}
		result = append(result, renderedMsg)
	}

	// Case 3: Add non-system messages while preserving order
	// Filter out system messages to prevent duplication since they're already processed above
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

// buildRequest converts the client request to a model request.
func (r *ClientRequest) buildRequest() (*Request, error) {
	msgs, err := r.getMessages()
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}

	req.Options = r.getOptions()
	req.Options.Tools = append(req.Options.Tools, r.tools...)
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call returns a caller for synchronous chat operations.
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{
		request: r,
	}
}

// Stream returns a streamer for streaming chat operations.
func (r *ClientRequest) Stream() *ClientStreamer {
	return &ClientStreamer{
		request: r,
	}
}

// ClientStreamer handles streaming chat operations.
type ClientStreamer struct {
	request *ClientRequest
}

// stream runs the streaming chat operation through the middleware chain.
func (s *ClientStreamer) stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return s.
		request.
		MiddlewareManager().
		BuildStreamHandler(s.request.model).
		Stream(ctx, req)
}

// response performs the streaming chat operation with optional structured parsing.
// Note: Structured parsing is not yet implemented for streaming due to data aggregation requirements.
func (s *ClientStreamer) response(ctx context.Context, parser StructuredParser[any]) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		req, err := s.request.buildRequest()
		if err != nil {
			yield(nil, err)
			return
		}

		if parser != nil {
			req.AppendToLastUserMessage(parser.Instructions())
		}

		for resp, streamErr := range s.stream(ctx, req) {
			if streamErr != nil {
				yield(nil, streamErr)
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

// ClientCaller handles synchronous chat operations.
type ClientCaller struct {
	request *ClientRequest
}

// call executes the synchronous chat operation through the middleware chain.
func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
	return c.
		request.
		MiddlewareManager().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

// response performs the chat operation with optional structured parsing.
func (c *ClientCaller) response(ctx context.Context, parser StructuredParser[any]) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}

	if parser != nil {
		req.AppendToLastUserMessage(parser.Instructions())
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

	resp, err := c.response(ctx, WrapParserAsAny(parser))
	if err != nil {
		return nil, nil, err
	}

	data, parseErr := parser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, parseErr
}

// Map executes the chat with map parsing and returns structured map data.
// Uses default map parser if none provided.
func (c *ClientCaller) Map(ctx context.Context, mapParser ...StructuredParser[map[string]any]) (map[string]any, *Response, error) {
	parser := pkgSlices.FirstOr(mapParser, nil)
	if parser == nil {
		parser = NewMapParser()
	}

	resp, err := c.response(ctx, WrapParserAsAny(parser))
	if err != nil {
		return nil, nil, err
	}

	data, parseErr := parser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, parseErr
}

// Any executes the chat with custom structured parsing.
func (c *ClientCaller) Any(ctx context.Context, anyParser StructuredParser[any]) (any, *Response, error) {
	resp, err := c.response(ctx, anyParser)
	if err != nil {
		return nil, nil, err
	}

	data, parseErr := anyParser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, parseErr
}

// Client provides a high-level interface for chat interactions
// with configurable defaults and fluent API support.
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient creates a new chat client with the specified default client request.
// Returns an error if request is nil.
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("client request cannot be nil")
	}

	return &Client{
		defaultRequest: request,
	}, nil
}

// NewClientWithModel creates a new chat client with the specified model.
// This is a convenience function that creates a default ClientRequest internally.
// Returns an error if model is nil or request creation fails.
func NewClientWithModel(model Model) (*Client, error) {
	request, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClient(request)
}

// Chat returns a new client request based on the client's defaults
// for creating customized chat interactions.
func (c *Client) Chat() *ClientRequest {
	return c.defaultRequest.Clone()
}

// ChatRequest creates a chat interaction from an existing request,
// merging its configuration with client defaults.
func (c *Client) ChatRequest(req *Request) *ClientRequest {
	return c.
		Chat().
		WithMessages(req.Messages...).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// ChatText creates a chat interaction with a simple text message
// using default options.
func (c *Client) ChatText(text string) *ClientRequest {
	return c.
		Chat().
		WithMessages(NewUserMessage(text))
}

// ChatPromptTemplate creates a chat interaction using a prompt template
// for the user message.
func (c *Client) ChatPromptTemplate(promptTemplate *PromptTemplate) *ClientRequest {
	return c.
		Chat().
		WithUserPromptTemplate(promptTemplate)
}
