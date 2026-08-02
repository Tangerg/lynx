package batching_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/internal/vectorstorekit/batching"
)

type batcherFunc func(context.Context, []*document.Document) ([][]*document.Document, error)

func (f batcherFunc) Batch(
	ctx context.Context,
	documents []*document.Document,
) ([][]*document.Document, error) {
	return f(ctx, documents)
}

func TestBatch_AcceptsOrderPreservingPartition(t *testing.T) {
	documents := []*document.Document{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	batches, err := batching.Batch(t.Context(), batcherFunc(func(
		_ context.Context,
		input []*document.Document,
	) ([][]*document.Document, error) {
		return [][]*document.Document{input[:2], input[2:]}, nil
	}), documents)
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("batch count = %d, want 2", len(batches))
	}
}

func TestBatch_RejectsInvalidPartitions(t *testing.T) {
	documents := []*document.Document{{ID: "a"}, {ID: "b"}}
	tests := map[string][][]*document.Document{
		"missing document": {documents[:1]},
		"reordered":        {{documents[1], documents[0]}},
		"duplicate":        {{documents[0], documents[0]}},
		"empty batch":      {documents[:1], nil, documents[1:]},
		"extra document":   {documents, {{ID: "c"}}},
	}
	for name, output := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := batching.Batch(t.Context(), batcherFunc(func(
				context.Context,
				[]*document.Document,
			) ([][]*document.Document, error) {
				return output, nil
			}), documents)
			if !errors.Is(err, batching.ErrInvalidOutput) {
				t.Fatalf("error = %v, want ErrInvalidOutput", err)
			}
		})
	}
}

func TestBatch_PreservesBatcherError(t *testing.T) {
	want := errors.New("batch failed")
	_, err := batching.Batch(t.Context(), batcherFunc(func(
		context.Context,
		[]*document.Document,
	) ([][]*document.Document, error) {
		return nil, want
	}), nil)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
