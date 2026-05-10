package embedding

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model"
)

// Type aliases threading embedding's *Request / *Response into the
// generic [model] handler/middleware machinery.
type (
	Handler           = model.CallHandler[*Request, *Response]
	HandlerFunc       = model.CallHandlerFunc[*Request, *Response]
	Middleware        = model.CallMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response, any, any]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// embedding's *Request / *Response pair. The stream side is unused
// (embedding has no stream endpoint).
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response, any, any]()
}

// ClientRequest is the fluent builder that turns a [Model] plus inputs
// and options into an embedding call. Construct one with
// [NewClientRequest] (or [Client.Embed] which clones the client's
// default), chain WithXxx methods, then finish with
// [ClientRequest.Call].
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	texts             []string
	params            map[string]any
}

// NewClientRequest builds a [ClientRequest] for model. Returns an error
// when model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("embedding.NewClientRequest: model must not be nil")
	}
	return &ClientRequest{model: model}, nil
}

// WithMiddlewares replaces the entire middleware chain.
func (r *ClientRequest) WithMiddlewares(middlewares ...Middleware) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseCallMiddlewares(middlewares...)
	}
	return r
}

// WithOptions sets the per-request [Options]. nil is ignored.
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithTexts replaces the input list. Empty input is ignored.
func (r *ClientRequest) WithTexts(texts []string) *ClientRequest {
	if len(texts) > 0 {
		r.texts = texts
	}
	return r
}

// WithParams replaces the side-channel params map. Empty input is ignored.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = params
	}
	return r
}

// MiddlewareManager returns the active manager, lazily allocating one
// if none has been set yet.
func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

// Clone returns a deep copy. Middleware chain, options, texts, and
// params are all duplicated so the caller can mutate independently.
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		texts:             slices.Clone(r.texts),
		params:            maps.Clone(r.params),
	}
}

// resolveOptions returns the effective [Options] for this call —
// request-level options when supplied, otherwise a clone of the model's
// defaults so the model's state stays untouched.
func (r *ClientRequest) resolveOptions() *Options {
	if r.options != nil {
		return r.options.Clone()
	}
	return r.model.DefaultOptions().Clone()
}

// buildRequest assembles the [*Request] sent through the middleware
// chain to the underlying model.
func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.texts)
	if err != nil {
		return nil, err
	}
	req.Options = r.resolveOptions()
	req.Params = maps.Clone(r.params)
	return req, nil
}

// Call returns a [ClientCaller] for executing the request.
//
// Example:
//
//	resp, err := client.Embed().WithTexts([]string{"hi"}).Call().Response(ctx)
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{request: r}
}

// ClientCaller drives the synchronous embedding path. Build it via
// [ClientRequest.Call]; finish via [ClientCaller.Response],
// [ClientCaller.Embedding], or [ClientCaller.Embeddings].
type ClientCaller struct {
	request *ClientRequest
}

// Response runs the call through the middleware chain and returns the
// raw [*Response].
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.request.
		MiddlewareManager().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
}

// Embedding runs the call and returns the first embedding vector
// alongside the full response — convenient for single-input requests.
func (c *ClientCaller) Embedding(ctx context.Context) ([]float64, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	return resp.Result().Embedding, resp, nil
}

// Embeddings runs the call and returns every embedding vector in input
// order — convenient for batch requests.
func (c *ClientCaller) Embeddings(ctx context.Context) ([][]float64, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([][]float64, 0, len(resp.Results))
	for _, result := range resp.Results {
		out = append(out, result.Embedding)
	}
	return out, resp, nil
}

// Client wraps a [Model] with a sticky default [ClientRequest], so each
// [Client.Embed] call clones a pre-configured starting point.
//
// Example:
//
//	client, err := embedding.NewClientWithModel(model)
//	v, _, err := client.EmbedWithText("hello").Call().Embedding(ctx)
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient wraps an existing [ClientRequest] as a sticky default.
// Returns an error when request is nil.
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("embedding.NewClient: request must not be nil")
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

// Embed returns a fresh clone of the default request, ready for fluent
// configuration.
func (c *Client) Embed() *ClientRequest {
	return c.defaultRequest.Clone()
}

// EmbedWithRequest seeds a clone with the texts, options, and params
// from req — useful when the caller already has an assembled [Request].
func (c *Client) EmbedWithRequest(req *Request) *ClientRequest {
	return c.Embed().
		WithTexts(req.Texts).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// EmbedWithText is the shorthand for a single-input request.
func (c *Client) EmbedWithText(text string) *ClientRequest {
	return c.EmbedWithTexts([]string{text})
}

// EmbedWithTexts is the shorthand for a batch request.
func (c *Client) EmbedWithTexts(texts []string) *ClientRequest {
	return c.Embed().WithTexts(texts)
}

// EmbedWithDocument is the document-shaped shorthand for
// [Client.EmbedWithText].
func (c *Client) EmbedWithDocument(doc *document.Document) *ClientRequest {
	return c.EmbedWithText(doc.Text)
}

// EmbedWithDocuments is the document-shaped shorthand for
// [Client.EmbedWithTexts].
func (c *Client) EmbedWithDocuments(docs []*document.Document) *ClientRequest {
	texts := make([]string, 0, len(docs))
	for _, doc := range docs {
		texts = append(texts, doc.Text)
	}
	return c.EmbedWithTexts(texts)
}
