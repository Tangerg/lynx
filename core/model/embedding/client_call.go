package embedding

import (
	"context"
	"errors"
	"time"
)

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
		MiddlewareChain().
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
