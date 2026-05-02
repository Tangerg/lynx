package chat

import (
	"context"
	"errors"
	"iter"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/model"
)

// Type aliases threading the chat-specific Request/Response into the
// generic [model] handler/middleware machinery.
type (
	CallHandler       = model.CallHandler[*Request, *Response]
	StreamHandler     = model.StreamHandler[*Request, *Response]
	CallHandlerFunc   = model.CallHandlerFunc[*Request, *Response]
	StreamHandlerFunc = model.StreamHandlerFunc[*Request, *Response]
	CallMiddleware    = model.CallMiddleware[*Request, *Response]
	StreamMiddleware  = model.StreamMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response, *Request, *Response]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// chat's *Request / *Response pair.
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response, *Request, *Response]()
}

// ClientRequest is the fluent builder that turns a [Model] plus a
// conversation, options, and middleware into a chat call. Construct one
// with [NewClientRequest] (or via [Client.Chat] which clones the
// client's default), then chain WithXxx methods, and finish with
// [ClientRequest.Call] for synchronous calls or [ClientRequest.Stream]
// for streaming.
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

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil — the request needs a backend to call.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("chat.NewClientRequest: model must not be nil")
	}

	return &ClientRequest{
		model:             model,
		middlewareManager: NewMiddlewareManager(),
		params:            make(map[string]any),
		tools:             make([]Tool, 0),
	}, nil
}

// WithMiddlewares replaces the entire middleware chain with the given
// values. Pass call middlewares, stream middlewares, or values returned
// by [NewToolMiddleware] which yields both at once.
func (r *ClientRequest) WithMiddlewares(middlewares ...any) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return r
}

// WithMiddlewareManager replaces the underlying [MiddlewareManager]. nil
// is ignored to preserve the current chain.
func (r *ClientRequest) WithMiddlewareManager(mgr *MiddlewareManager) *ClientRequest {
	if mgr != nil {
		r.middlewareManager = mgr
	}
	return r
}

// WithOptions sets the per-request [Options]. nil is ignored — the model
// default still applies.
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithUserPrompt installs a fresh [PromptTemplate] for the user message
// from a raw template string. Empty input is ignored.
func (r *ClientRequest) WithUserPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.userPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return r
}

// WithUserPromptTemplate installs the given user-prompt template.
// nil is ignored.
func (r *ClientRequest) WithUserPromptTemplate(template *PromptTemplate) *ClientRequest {
	if template != nil {
		r.userPromptTemplate = template
	}
	return r
}

// WithSystemPrompt installs a fresh [PromptTemplate] for the system
// message from a raw template string. Empty input is ignored.
func (r *ClientRequest) WithSystemPrompt(prompt string) *ClientRequest {
	if prompt != "" {
		r.systemPromptTemplate = NewPromptTemplate().WithTemplate(prompt)
	}
	return r
}

// WithSystemPromptTemplate installs the given system-prompt template.
// nil is ignored.
func (r *ClientRequest) WithSystemPromptTemplate(template *PromptTemplate) *ClientRequest {
	if template != nil {
		r.systemPromptTemplate = template
	}
	return r
}

// WithMessages replaces the conversation with the given messages.
// Empty input is ignored.
func (r *ClientRequest) WithMessages(messages ...Message) *ClientRequest {
	if len(messages) > 0 {
		r.messages = messages
	}
	return r
}

// WithParams replaces the side-channel params map. Use it to thread
// trace ids, user ids, and other middleware-readable values. Empty
// input is ignored.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = params
	}
	return r
}

// WithTools attaches the given tools for this request. Replaces any
// previously-attached tools.
func (r *ClientRequest) WithTools(tools ...Tool) *ClientRequest {
	if len(tools) > 0 {
		r.tools = tools
	}
	return r
}

// MiddlewareManager returns the active manager, lazily allocating one
// if [WithMiddlewares] / [WithMiddlewareManager] has not run yet.
func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

// Clone returns a deep copy of the request — middleware chain, options,
// templates, message slice, params, and tools are all duplicated so the
// caller can mutate the clone independently of the original.
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

// resolveOptions returns the effective [Options] for this call —
// request-level options when supplied, otherwise a clone of the model's
// defaults so the caller never mutates the model's state.
func (r *ClientRequest) resolveOptions() *Options {
	if r.options != nil {
		return r.options.Clone()
	}
	return r.model.DefaultOptions().Clone()
}

// resolveMessages produces the final, normalized message list for the
// model. The flow is:
//
//  1. If messages is empty, seed with the user-prompt template (or a
//     fallback "Hi!" so the conversation has something to say).
//  2. Pull system messages: existing ones merge first; otherwise render
//     the system-prompt template.
//  3. Append the non-system messages (user / assistant / tool) in
//     original order.
//  4. Fold runs of adjacent same-type messages into single merged
//     messages — the planner's preferred shape.
func (r *ClientRequest) resolveMessages() ([]Message, error) {
	msgs := slices.Clone(r.messages)

	if len(msgs) == 0 {
		seed, err := r.seedMessage()
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, seed)
	}

	out := make([]Message, 0, len(msgs)+1)

	// System message: prefer existing (merged) over template-rendered.
	if sys := MergeSystemMessages(msgs); sys != nil {
		out = append(out, sys)
	} else if r.systemPromptTemplate != nil {
		rendered, err := r.systemPromptTemplate.CreateSystemMessage()
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}

	// Non-system messages keep their original order.
	out = append(out, FilterMessagesByMessageTypes(msgs, MessageTypeUser, MessageTypeAssistant, MessageTypeTool)...)

	return MergeAdjacentSameTypeMessages(out), nil
}

// seedMessage produces the first user turn when the caller didn't
// supply any messages — either the user-prompt template, or a friendly
// fallback so the conversation has something to start from.
func (r *ClientRequest) seedMessage() (Message, error) {
	if r.userPromptTemplate != nil {
		return r.userPromptTemplate.CreateUserMessage()
	}
	return NewUserMessage("Hi!"), nil
}

// buildRequest assembles the [Request] sent through the middleware chain
// to the underlying model.
func (r *ClientRequest) buildRequest() (*Request, error) {
	msgs, err := r.resolveMessages()
	if err != nil {
		return nil, err
	}

	req, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}

	req.Options = r.resolveOptions()
	req.Options.Tools = append(req.Options.Tools, r.tools...)
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call returns a [ClientCaller] for synchronous Q&A.
//
// Example:
//
//	resp, err := client.Chat().WithText("hi").Call().Response(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// Stream returns a [ClientStreamer] for incremental output.
//
// Example:
//
//	for chunk, err := range client.Chat().WithText("hi").Stream().Text(ctx) {
//	    if err != nil { return err }
//	    fmt.Print(chunk)
//	}
func (r *ClientRequest) Stream() *ClientStreamer {
	return &ClientStreamer{request: r}
}

// ClientStreamer drives the streaming chat path. Build it via
// [ClientRequest.Stream]; consume it via [ClientStreamer.Response] or
// [ClientStreamer.Text].
type ClientStreamer struct {
	request *ClientRequest
}

// stream feeds the request through the middleware chain into the model.
// Tool execution is NOT auto-injected — register [NewToolMiddleware] via
// WithMiddlewares if you need that.
func (s *ClientStreamer) stream(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	handler := s.request.MiddlewareManager().BuildStreamHandler(s.request.model)
	return handler.Stream(ctx, req)
}

// runStream is the shared entry point for streaming: build the request,
// optionally inject parser instructions, then run the middleware chain.
// Structured parsing on streams is not yet implemented (the parser
// requires the full text and stream provides incremental chunks).
func (s *ClientStreamer) runStream(ctx context.Context, parser StructuredParser[any]) iter.Seq2[*Response, error] {
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

// Response streams full [*Response] chunks as they arrive.
func (s *ClientStreamer) Response(ctx context.Context) iter.Seq2[*Response, error] {
	return s.runStream(ctx, nil)
}

// Text streams just the assistant's text deltas — convenient when you
// want to write directly to a UI buffer without unpacking the response.
func (s *ClientStreamer) Text(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for resp, err := range s.runStream(ctx, nil) {
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

// ClientCaller drives the synchronous chat path. Build it via
// [ClientRequest.Call]; finish via [ClientCaller.Response],
// [ClientCaller.Text], or [ClientCaller.Structured].
type ClientCaller struct {
	request *ClientRequest
}

// call feeds the request through the middleware chain into the model.
// Tool execution is NOT auto-injected — register [NewToolMiddleware] via
// WithMiddlewares if you need that.
func (c *ClientCaller) call(ctx context.Context, req *Request) (*Response, error) {
	handler := c.request.MiddlewareManager().BuildCallHandler(c.request.model)
	return handler.Call(ctx, req)
}

// runCall is the shared entry point: build the request, optionally
// inject parser instructions, then call.
func (c *ClientCaller) runCall(ctx context.Context, parser StructuredParser[any]) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}

	if parser != nil {
		req.AppendToLastUserMessage(parser.Instructions())
	}

	return c.call(ctx, req)
}

// Response runs the call and returns the raw [*Response].
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	return c.runCall(ctx, nil)
}

// Text runs the call and returns the assistant's plain-text reply
// alongside the full response (kept so callers can still inspect
// usage / metadata).
func (c *ClientCaller) Text(ctx context.Context) (string, *Response, error) {
	resp, err := c.runCall(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	return resp.Result().AssistantMessage.Text, resp, nil
}

// Structured runs the call with parser-supplied prompt instructions
// then decodes the assistant's text into the parser's typed value.
//
// Example:
//
//	parser := chat.NewJSONParser[Recipe]()
//	any, _, err := client.Chat().WithText("...").Call().Structured(ctx, chat.WrapParserAsAny(parser))
func (c *ClientCaller) Structured(ctx context.Context, parser StructuredParser[any]) (any, *Response, error) {
	resp, err := c.runCall(ctx, parser)
	if err != nil {
		return nil, nil, err
	}
	data, parseErr := parser.Parse(resp.Result().AssistantMessage.Text)
	return data, resp, parseErr
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Chat] call clones a pre-configured starting point. Construct
// one with [NewClientWithModel] for the simple case, or [NewClient] when
// you want to install default middlewares / options on the underlying
// request.
//
// Example:
//
//	client, err := chat.NewClientWithModel(model)
//	resp, err := client.Chat().WithText("hi").Call().Response(ctx)
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient wraps an existing [ClientRequest] as a sticky default.
// Returns an error when request is nil.
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("chat.NewClient: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

// NewClientWithModel is a one-step constructor: build a default
// [ClientRequest] for model, then wrap it as a [Client].
func NewClientWithModel(model Model) (*Client, error) {
	req, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClient(req)
}

// Chat returns a fresh clone of the default request, ready for fluent
// configuration without affecting the client's defaults.
func (c *Client) Chat() *ClientRequest {
	return c.defaultRequest.Clone()
}

// ChatWithRequest seeds a clone with the messages, options, and params
// from req — useful when the caller already has an assembled [Request]
// (e.g. forwarded from another service).
func (c *Client) ChatWithRequest(req *Request) *ClientRequest {
	return c.Chat().
		WithMessages(req.Messages...).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// ChatWithText is the most common shortcut: a single user-message turn.
func (c *Client) ChatWithText(text string) *ClientRequest {
	return c.Chat().WithMessages(NewUserMessage(text))
}

// ChatWithPrompt is an alias for [Client.ChatWithText].
func (c *Client) ChatWithPrompt(prompt string) *ClientRequest {
	return c.ChatWithText(prompt)
}

// ChatWithPromptTemplate seeds a clone with the given user-prompt
// template — render it later via [ClientCaller] / [ClientStreamer].
func (c *Client) ChatWithPromptTemplate(template *PromptTemplate) *ClientRequest {
	return c.Chat().WithUserPromptTemplate(template)
}
