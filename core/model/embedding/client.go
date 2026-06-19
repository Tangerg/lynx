package embedding

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model"
)

type (
	Handler           = model.CallHandler[*Request, *Response]
	HandlerFunc       = model.CallHandlerFunc[*Request, *Response]
	Middleware        = model.CallMiddleware[*Request, *Response]
	MiddlewareManager = model.MiddlewareManager[*Request, *Response]
)

// NewMiddlewareManager returns an empty [MiddlewareManager] keyed to
// embedding's *Request / *Response pair. The stream side is unused
// (embedding has no stream endpoint).
func NewMiddlewareManager() *MiddlewareManager {
	return model.NewMiddlewareManager[*Request, *Response]()
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

// WithTexts replaces the input list. Empty input is ignored. The
// slice is cloned so caller mutations don't leak into the request.
func (r *ClientRequest) WithTexts(texts []string) *ClientRequest {
	if len(texts) > 0 {
		r.texts = slices.Clone(texts)
	}
	return r
}

// WithParams replaces the side-channel params map. Empty input is
// ignored. The map is cloned so caller mutations don't leak.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = maps.Clone(params)
	}
	return r
}

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

func (r *ClientRequest) resolveOptions() *Options {
	if r.options != nil {
		return r.options.Clone()
	}
	defaults := r.model.DefaultOptions()
	return defaults.Clone()
}

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

type ClientCaller struct {
	request *ClientRequest
}

// Response runs the call through the middleware chain and returns the
// raw [*Response].
//
// One OTel span is started per call following the GenAI semconv —
// see [startEmbeddingSpan] / [finishEmbeddingSpan] for the attribute
// set. No-op overhead when no TracerProvider is configured.
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	start := time.Now()
	ctx, span := startEmbeddingSpan(ctx, c.request.model, req)
	resp, err := c.request.
		MiddlewareManager().
		BuildCallHandler(c.request.model).
		Call(ctx, req)
	finishEmbeddingSpan(span, resp, err)
	recordEmbeddingMetrics(ctx, c.request.model, req, resp, err, start)
	return resp, err
}

func (c *ClientCaller) Embedding(ctx context.Context) ([]float64, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}
	// Providers aren't forced to populate Results — guard rather than panic.
	result := resp.Result()
	if result == nil {
		return nil, resp, errors.New("embedding.ClientCaller.Embedding: response carries no results")
	}
	return result.Embedding, resp, nil
}

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
//	client, err := embedding.NewClient(model)
//	v, _, err := client.EmbedWithText("hello").Call().Embedding(ctx)
type Client struct {
	defaultRequest *ClientRequest
}

func NewClient(model Model) (*Client, error) {
	req, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClientFromRequest(req)
}

// NewClientFromRequest wraps an existing [ClientRequest] as a sticky
// default — use this when the request already carries default
// middlewares / options the [Client] should keep applying.
func NewClientFromRequest(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("embedding.NewClientFromRequest: request must not be nil")
	}
	return &Client{defaultRequest: request}, nil
}

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

func (c *Client) EmbedWithText(text string) *ClientRequest {
	return c.EmbedWithTexts([]string{text})
}

func (c *Client) EmbedWithTexts(texts []string) *ClientRequest {
	return c.Embed().WithTexts(texts)
}

func (c *Client) EmbedWithDocument(doc *document.Document) *ClientRequest {
	return c.EmbedWithText(doc.Text)
}

func (c *Client) EmbedWithDocuments(docs []*document.Document) *ClientRequest {
	texts := make([]string, 0, len(docs))
	for _, doc := range docs {
		texts = append(texts, doc.Text)
	}
	return c.EmbedWithTexts(texts)
}
