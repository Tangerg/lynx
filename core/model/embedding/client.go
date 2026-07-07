package embedding

import (
	"errors"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model"
)

type (
	Handler         = model.CallHandler[*Request, *Response]
	HandlerFunc     = model.CallHandlerFunc[*Request, *Response]
	Middleware      = model.CallMiddleware[*Request, *Response]
	MiddlewareChain = model.MiddlewareChain[*Request, *Response]
)

// NewMiddlewareChain returns an empty [MiddlewareChain] keyed to
// embedding's *Request / *Response pair. The stream side is unused
// (embedding has no stream endpoint).
func NewMiddlewareChain() MiddlewareChain {
	return model.NewMiddlewareChain[*Request, *Response]()
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
