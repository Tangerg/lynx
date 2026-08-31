package embeddingclient

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/embedding"
)

var ErrNilModel = errors.New("embeddingclient: nil model")

// Client is an immutable, concurrency-safe projection of [embedding.Model] for
// callers that need vectors rather than protocol responses. It validates both
// model boundaries, preserves input order, and returns independently owned
// vectors. Provider metadata remains available only through the model.
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

// EmbedText embeds one text without exposing batch cardinality to a caller that
// has exactly one input.
func (c Client) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := c.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (c Client) validate() error {
	if lo.IsNil(c.model) {
		return ErrNilModel
	}
	return nil
}
