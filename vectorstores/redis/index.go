package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
)

// Index embeds documents and writes them as Redis HASHes keyed by
// `<KeyPrefix><id>`.
func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if validateErr := request.Validate(); validateErr != nil {
		return fmt.Errorf("redis.Store.Index: %w", validateErr)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("redis: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("redis: embed documents: %w", err)
		}

		pipe := s.client.Pipeline()
		for i, doc := range docs {
			id := doc.ID
			metadataValues, valuesErr := doc.Metadata.Values()
			if valuesErr != nil {
				return fmt.Errorf("redis: decode metadata for %s: %w", id, valuesErr)
			}
			fields := map[string]any{
				s.contentField:   doc.Text,
				s.embeddingField: float32sToBytes(embedding.Float32Vector(vectors[i])),
			}
			for k, v := range metadataValues {
				fields[k] = formatMetadataValue(v)
			}
			pipe.HSet(ctx, s.keyPrefix+id, fields)
		}

		if _, err = pipe.Exec(ctx); err != nil {
			return fmt.Errorf("redis: pipeline HSET: %w", err)
		}
	}
	return nil
}

// formatMetadataValue coerces a Go value into the HASH string form
// RediSearch can index. Slices and maps are JSON-encoded — they only
// matter when the caller stored them as TEXT fields.
func formatMetadataValue(v any) any {
	switch val := v.(type) {
	case nil:
		return ""
	case string, int, int64, float32, float64, bool:
		return val
	case []byte:
		return val
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(b)
	}
}
