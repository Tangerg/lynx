package chat

import (
	"errors"
	"maps"
	"slices"
)

// ClientRequest is the fluent builder that turns a [Model] plus a
// conversation, options, and middleware into a chat call. Construct one
// with [NewClientRequest] (or via [Client.Chat] which clones the
// client's default), then chain WithXxx methods, and finish with
// [ClientRequest.Call] for synchronous calls or [ClientRequest.Stream]
// for streaming.
type ClientRequest struct {
	model                Model
	middlewares          MiddlewareChain
	options              *Options
	userPromptTemplate   *PromptTemplate
	systemPromptTemplate *PromptTemplate
	messages             []Message
	params               map[string]any
	tools                []Tool
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil because the request needs a backend to call.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("chat.NewClientRequest: model must not be nil")
	}

	return &ClientRequest{
		model:       model,
		middlewares: NewMiddlewareChain(),
		params:      make(map[string]any),
		tools:       make([]Tool, 0),
	}, nil
}

// WithMiddlewareChain replaces the full middleware chain.
func (r *ClientRequest) WithMiddlewareChain(chain MiddlewareChain) *ClientRequest {
	r.middlewares = chain.Clone()
	return r
}

// WithCallMiddlewares replaces the call-side middleware chain.
func (r *ClientRequest) WithCallMiddlewares(middlewares ...CallMiddleware) *ClientRequest {
	r.middlewares = r.middlewares.WithCall(middlewares...)
	return r
}

// WithStreamMiddlewares replaces the stream-side middleware chain.
func (r *ClientRequest) WithStreamMiddlewares(middlewares ...StreamMiddleware) *ClientRequest {
	r.middlewares = r.middlewares.WithStream(middlewares...)
	return r
}

// WithOptions sets the per-request [Options]. nil is ignored, so the
// model default still applies.
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
		r.userPromptTemplate = NewPromptTemplate(prompt)
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
		r.systemPromptTemplate = NewPromptTemplate(prompt)
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
// Empty input is ignored. The slice is cloned so caller mutations
// don't leak into the request.
func (r *ClientRequest) WithMessages(messages ...Message) *ClientRequest {
	if len(messages) > 0 {
		r.messages = slices.Clone(messages)
	}
	return r
}

// WithParams replaces the side-channel params map. Use it to thread
// trace ids, user ids, and other middleware-readable values. Empty
// input is ignored. The map is cloned so caller mutations don't leak.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = maps.Clone(params)
	}
	return r
}

// WithTools attaches the given tools for this request. Replaces any
// previously-attached tools. The slice is cloned so caller mutations
// don't leak.
func (r *ClientRequest) WithTools(tools ...Tool) *ClientRequest {
	if len(tools) > 0 {
		r.tools = slices.Clone(tools)
	}
	return r
}

// MiddlewareChain returns a defensive copy of the active chain.
func (r *ClientRequest) MiddlewareChain() MiddlewareChain {
	return r.middlewares.Clone()
}

// Clone returns a deep copy of the request. Middleware chain, options,
// templates, message slice, params, and tools are all duplicated so the
// caller can mutate the clone independently of the original.
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:                r.model,
		middlewares:          r.middlewares.Clone(),
		options:              r.options.Clone(),
		userPromptTemplate:   r.userPromptTemplate.Clone(),
		systemPromptTemplate: r.systemPromptTemplate.Clone(),
		messages:             slices.Clone(r.messages),
		params:               maps.Clone(r.params),
		tools:                slices.Clone(r.tools),
	}
}

func (r *ClientRequest) resolveOptions() *Options {
	defaults := r.model.DefaultOptions()
	if r.options != nil {
		merged, err := MergeOptions(&defaults, r.options)
		if err == nil {
			return merged
		}
	}
	return defaults.Clone()
}

// resolveMessages produces the final, normalized message list. It seeds
// from the user-prompt template if empty, renders the system prompt into
// a leading system message, then merges adjacent same-type runs.
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
	list := MessageList(msgs)

	if sys := list.MergeSystem(); sys != nil {
		out = append(out, sys)
	} else if r.systemPromptTemplate != nil {
		rendered, err := r.systemPromptTemplate.CreateSystemMessage()
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}

	out = append(out, list.FilterTypes(MessageTypeUser, MessageTypeAssistant, MessageTypeTool)...)

	return MessageList(out).MergeAdjacentSameType(), nil
}

func (r *ClientRequest) seedMessage() (Message, error) {
	if r.userPromptTemplate != nil {
		return r.userPromptTemplate.CreateUserMessage()
	}
	return nil, errors.New("chat.ClientRequest: request must carry at least one message or a user-prompt template")
}

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
	req.Tools = slices.Clone(r.tools)
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call returns a [ClientCaller] for synchronous Q&A.
//
// Example:
//
//	resp, err := client.Chat().WithUserPrompt("hi").Call().Response(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// Stream returns a [ClientStreamer] for incremental output.
//
// Example:
//
//	for chunk, err := range client.Chat().WithUserPrompt("hi").Stream().Text(ctx) {
//	    if err != nil { return err }
//	    fmt.Print(chunk)
//	}
func (r *ClientRequest) Stream() *ClientStreamer {
	return &ClientStreamer{request: r}
}
