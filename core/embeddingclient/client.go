package embeddingclient

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/embedding"
)

// ErrNilModel reports that a Client has no usable model, including a typed nil.
var ErrNilModel = errors.New("embeddingclient: nil model")

// Client is an immutable, concurrency-safe projection of [embedding.Model] for
// callers that need vectors rather than protocol responses. It validates both
// model boundaries, preserves input order, and returns independently owned
// vectors. Dimensions deliberately probes once per call instead of hiding a
// cache lifetime; provider metadata remains available only through the model.
type Client struct {
	model embedding.Model
}

func New(model embedding.Model) (Client, error) {
	client := Client{model: model}
	if err := client.validate(); err != nil {
		return Client{}, err
	}
	return client, nil
}

func (c Client) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("embeddingclient: embed texts: %w", err)
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
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("embeddingclient: embed texts: invalid model response: %w", err)
	}
	if len(response.Outputs) != len(texts) {
		return nil, fmt.Errorf("embeddingclient: embed texts: got %d outputs for %d inputs", len(response.Outputs), len(texts))
	}

	vectors := make([][]float64, len(response.Outputs))
	for i, output := range response.Outputs {
		vectors[i] = slices.Clone(output.Embedding)
	}
	return vectors, nil
}

func (c Client) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (c Client) Dimensions(ctx context.Context) (int, error) {
	vector, err := c.EmbedText(ctx, "dimension probe")
	if err != nil {
		return 0, fmt.Errorf("embeddingclient: dimensions: %w", err)
	}
	return len(vector), nil
}

func (c Client) EmbedDocuments(ctx context.Context, docs []*document.Document) ([][]float64, error) {
	texts, err := c.documentTexts(docs)
	if err != nil {
		return nil, err
	}
	return c.EmbedTexts(ctx, texts)
}

func (Client) documentTexts(docs []*document.Document) ([]string, error) {
	if len(docs) == 0 {
		return nil, errors.New("embeddingclient: embed documents: documents must not be empty")
	}
	texts := make([]string, len(docs))
	for i, doc := range docs {
		if doc == nil {
			return nil, fmt.Errorf("embeddingclient: embed documents: document %d is nil", i)
		}
		if err := doc.Validate(); err != nil {
			return nil, fmt.Errorf("embeddingclient: embed documents: document %d is invalid: %w", i, err)
		}
		if doc.Text == "" {
			return nil, fmt.Errorf("embeddingclient: embed documents: document %d has no text", i)
		}
		texts[i] = doc.Text
	}
	return texts, nil
}

func (c Client) validate() error {
	if c.model == nil || lo.IsNil(c.model) {
		return ErrNilModel
	}
	return nil
}
