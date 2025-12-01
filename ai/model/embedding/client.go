package embedding

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model"
)

type Handler = model.CallHandler[*Request, *Response]
type HandlerFunc = model.CallHandlerFunc[*Request, *Response]
type Middleware = model.CallMiddleware[*Request, *Response]
type MiddlewareManager = model.CallMiddlewareManager[*Request, *Response]

func NewMiddlewareManager() *MiddlewareManager {
	return model.NewCallMiddlewareManager[*Request, *Response]()
}

// ClientRequest represents a builder for configuring and executing embedding requests.
// It provides a fluent API for setting up the model, middlewares, options, texts, and parameters.
type ClientRequest struct {
	model             Model
	middlewareManager *MiddlewareManager
	options           *Options
	texts             []string
	params            map[string]any
}

// NewClientRequest creates a new client request with the specified embedding model.
// Returns an error if the model is nil.
func NewClientRequest(model Model) (*ClientRequest, error) {
	if model == nil {
		return nil, errors.New("embedding model cannot be nil")
	}
	return &ClientRequest{
		model: model,
	}, nil
}

// WithMiddlewares sets the middleware chain for the request, replacing any existing middlewares.
// Middlewares are executed in the order they are provided.
func (r *ClientRequest) WithMiddlewares(middlewares ...Middleware) *ClientRequest {
	if len(middlewares) > 0 {
		r.middlewareManager = NewMiddlewareManager().UseMiddlewares(middlewares...)
	}
	return r
}

// WithMiddlewareManager sets a custom middleware manager for the request.
// This allows for more advanced middleware configuration.
func (r *ClientRequest) WithMiddlewareManager(middlewareManager *MiddlewareManager) *ClientRequest {
	if middlewareManager != nil {
		r.middlewareManager = middlewareManager
	}
	return r
}

// WithOptions sets the configuration options for the embedding request.
// These options control aspects like encoding format and dimensions.
func (r *ClientRequest) WithOptions(options *Options) *ClientRequest {
	if options != nil {
		r.options = options
	}
	return r
}

// WithTexts sets the input texts to be embedded.
// Each input string will be converted into an embedding vector.
func (r *ClientRequest) WithTexts(inputs []string) *ClientRequest {
	if len(inputs) > 0 {
		r.texts = inputs
	}
	return r
}

// WithParams sets request-level parameters such as userID, sessionID, etc.
// These parameters are useful for tracking and logging but don't affect embedding generation.
func (r *ClientRequest) WithParams(params map[string]any) *ClientRequest {
	if len(params) > 0 {
		r.params = params
	}
	return r
}

// MiddlewareManager returns the middleware manager, initializing a new one if needed.
func (r *ClientRequest) MiddlewareManager() *MiddlewareManager {
	if r.middlewareManager == nil {
		r.middlewareManager = NewMiddlewareManager()
	}
	return r.middlewareManager
}

// Clone creates a deep copy of the client request.
// This is useful for creating multiple requests based on a common configuration.
func (r *ClientRequest) Clone() *ClientRequest {
	return &ClientRequest{
		model:             r.model,
		middlewareManager: r.middlewareManager.Clone(),
		options:           r.options.Clone(),
		texts:             slices.Clone(r.texts),
		params:            maps.Clone(r.params),
	}
}

// getOptions returns the effective options for the embedding request,
// merging request-specific options with model default settings.
func (r *ClientRequest) getOptions() *Options {
	var opts *Options

	if r.options != nil {
		opts = r.options.Clone()
	} else {
		opts = r.model.DefaultOptions().Clone()
	}

	return opts
}

// buildRequest constructs the final Request object from the client request configuration.
// Returns an error if the request cannot be built (e.g., no texts provided).
func (r *ClientRequest) buildRequest() (*Request, error) {
	req, err := NewRequest(r.texts)
	if err != nil {
		return nil, err
	}

	req.Options = r.getOptions()
	req.Params = maps.Clone(r.params)

	return req, nil
}

// Call prepares the request for execution and returns a ClientCaller for making the actual API call.
func (r *ClientRequest) Call() *ClientCaller {
	return &ClientCaller{
		request: r,
	}
}

// ClientCaller handles the execution of embedding requests and provides methods
// for retrieving results in different formats.
type ClientCaller struct {
	request *ClientRequest
}

// Response executes the embedding request and returns the complete response.
// This includes all embedding results and metadata.
func (c *ClientCaller) Response(ctx context.Context) (*Response, error) {
	req, err := c.request.buildRequest()
	if err != nil {
		return nil, err
	}
	return c.
		request.
		MiddlewareManager().
		BuildHandler(c.request.model).
		Call(ctx, req)
}

// Embedding executes the request and returns only the first embedding vector.
// This is a convenience method for single-input requests.
// Returns the embedding vector, the full response, and any error encountered.
func (c *ClientCaller) Embedding(ctx context.Context) ([]float64, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}

	return resp.Result().Embedding, resp, nil
}

// Embeddings executes the request and returns all embedding vectors.
// This is useful for batch embedding requests with multiple texts.
// Returns a slice of embedding vectors, the full response, and any error encountered.
func (c *ClientCaller) Embeddings(ctx context.Context) ([][]float64, *Response, error) {
	resp, err := c.Response(ctx)
	if err != nil {
		return nil, nil, err
	}

	embeddings := make([][]float64, 0, len(resp.Results))
	for _, result := range resp.Results {
		embeddings = append(embeddings, result.Embedding)
	}
	return embeddings, resp, nil
}

// Client provides a high-level interface for making embedding requests.
// It maintains a default configuration that can be cloned and customized for each request.
type Client struct {
	defaultRequest *ClientRequest
}

// NewClient creates a new embedding client with the specified default request configuration.
// Returns an error if the request is nil.
func NewClient(request *ClientRequest) (*Client, error) {
	if request == nil {
		return nil, errors.New("client request is required")
	}
	return &Client{
		defaultRequest: request,
	}, nil
}

// NewClientWithModel creates a new chat client with the specified model.
// This is a convenience function that creates a default ClientRequest internally.
// Returns an error if model is nil or request creation fails.
func NewClientWithModel(model Model) (*Client, error) {
	cliReq, err := NewClientRequest(model)
	if err != nil {
		return nil, err
	}
	return NewClient(cliReq)
}

// Embed returns a cloned client request based on the default configuration.
// This allows for further customization before execution.
func (c *Client) Embed() *ClientRequest {
	return c.defaultRequest.Clone()
}

// EmbedWithRequest creates a client request from an existing Request object.
// This allows for reusing previously configured requests with the client's settings.
func (c *Client) EmbedWithRequest(req *Request) *ClientRequest {
	return c.
		Embed().
		WithTexts(req.Texts).
		WithOptions(req.Options).
		WithParams(req.Params)
}

// EmbedWithText creates a request to embed a single text string.
// This is a convenience method that combines cloning and setting the input.
func (c *Client) EmbedWithText(text string) *ClientRequest {
	return c.EmbedWithTexts([]string{text})
}

// EmbedWithTexts creates a request to embed multiple text strings.
// This is a convenience method for batch embedding operations.
func (c *Client) EmbedWithTexts(texts []string) *ClientRequest {
	return c.
		Embed().
		WithTexts(texts)
}

// EmbedWithDocument creates a client request for embedding a single document.
// The document's text content will be used as the embedding input.
func (c *Client) EmbedWithDocument(doc *document.Document) *ClientRequest {
	return c.EmbedWithText(doc.Text)
}

// EmbedWithDocuments creates a client request for embedding multiple documents.
// Each document's text content will be extracted and embedded in order.
func (c *Client) EmbedWithDocuments(docs []*document.Document) *ClientRequest {
	contents := make([]string, 0, len(docs))
	for _, doc := range docs {
		contents = append(contents, doc.Text)
	}
	return c.EmbedWithTexts(contents)
}
