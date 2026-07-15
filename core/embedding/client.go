package embedding

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/core/document"
)

// Client is a stateless convenience wrapper around [Model]. It owns no
// defaults or middleware; provider defaults stay in provider construction and
// per-call overrides stay in [Request.Options].
type Client struct {
	model Model
}

func NewClient(model Model) (*Client, error) {
	if model == nil || isNilModel(model) {
		return nil, errors.New("embedding.NewClient: model must not be nil")
	}
	return &Client{model: model}, nil
}

func isNilModel(model Model) bool {
	value := reflect.ValueOf(model)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Call validates and sends request to the model.
func (c *Client) Call(ctx context.Context, request *Request) (*Response, error) {
	if c == nil || c.model == nil {
		return nil, errors.New("embedding.Client.Call: client is nil")
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("embedding.Client.Call: %w", err)
	}
	response, err := c.model.Call(ctx, request)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("embedding.Client.Call: model returned a nil response")
	}
	return response, nil
}

// EmbedTexts embeds all texts in one model call and returns vectors in input
// order. A provider response with missing or extra results is rejected.
func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float64, *Response, error) {
	request, err := NewRequest(texts)
	if err != nil {
		return nil, nil, err
	}
	response, err := c.Call(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	if len(response.Results) != len(texts) {
		return nil, response, fmt.Errorf("embedding.Client.EmbedTexts: got %d results for %d inputs", len(response.Results), len(texts))
	}
	vectors := make([][]float64, len(response.Results))
	for i, result := range response.Results {
		if result == nil || len(result.Embedding) == 0 {
			return nil, response, fmt.Errorf("embedding.Client.EmbedTexts: result %d has no embedding", i)
		}
		vectors[i] = slices.Clone(result.Embedding)
	}
	return vectors, response, nil
}

func (c *Client) EmbedText(ctx context.Context, text string) ([]float64, *Response, error) {
	vectors, response, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, response, err
	}
	return vectors[0], response, nil
}

func (c *Client) EmbedDocuments(ctx context.Context, docs []*document.Document) ([][]float64, *Response, error) {
	if len(docs) == 0 {
		return nil, nil, errors.New("embedding.Client.EmbedDocuments: documents must not be empty")
	}
	texts := make([]string, len(docs))
	for i, doc := range docs {
		if doc == nil {
			return nil, nil, fmt.Errorf("embedding.Client.EmbedDocuments: document %d is nil", i)
		}
		texts[i] = doc.Text
	}
	return c.EmbedTexts(ctx, texts)
}
