package embeddingclient

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
)

// ErrNilModel reports that [New] received a nil model, including a typed nil.
var ErrNilModel = errors.New("embeddingclient: nil model")

var errNilClient = errors.New("embeddingclient: nil client")

// Client is an immutable convenience wrapper around an [embedding.Model]. It
// returns independent vector values and leaves provider response metadata to
// callers that use the Core model directly.
type Client struct {
	model embedding.Model
}

// New constructs a Client around model.
func New(model embedding.Model) (*Client, error) {
	if model == nil || isNilModel(model) {
		return nil, ErrNilModel
	}
	return &Client{model: model}, nil
}

func isNilModel(model embedding.Model) bool {
	value := reflect.ValueOf(model)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// EmbedTexts embeds texts in one model call and returns vectors in input order.
func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if c == nil || c.model == nil {
		return nil, errNilClient
	}
	request, err := embedding.NewRequest(texts)
	if err != nil {
		return nil, fmt.Errorf("embeddingclient: embed texts: %w", err)
	}
	response, err := c.model.Call(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("embeddingclient: embed texts: %w", err)
	}
	if response == nil {
		return nil, errors.New("embeddingclient: embed texts: model returned a nil response")
	}
	if len(response.Results) != len(texts) {
		return nil, fmt.Errorf("embeddingclient: embed texts: got %d results for %d inputs", len(response.Results), len(texts))
	}

	vectors := make([][]float64, len(response.Results))
	for i, result := range response.Results {
		if result == nil || len(result.Embedding) == 0 {
			return nil, fmt.Errorf("embeddingclient: embed texts: result %d has no embedding", i)
		}
		vectors[i] = slices.Clone(result.Embedding)
	}
	return vectors, nil
}

// EmbedText embeds one text value.
func (c *Client) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

// EmbedDocuments embeds the textual content of docs in one model call.
func (c *Client) EmbedDocuments(ctx context.Context, docs []*document.Document) ([][]float64, error) {
	if len(docs) == 0 {
		return nil, errors.New("embeddingclient: embed documents: documents must not be empty")
	}
	texts := make([]string, len(docs))
	for i, doc := range docs {
		if doc == nil {
			return nil, fmt.Errorf("embeddingclient: embed documents: document %d is nil", i)
		}
		if doc.Text == "" {
			return nil, fmt.Errorf("embeddingclient: embed documents: document %d has no text", i)
		}
		texts[i] = doc.Text
	}
	return c.EmbedTexts(ctx, texts)
}
