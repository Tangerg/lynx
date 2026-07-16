package documentpipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/documentpipeline"
)

type textLengthEstimator struct{}

func (textLengthEstimator) EstimateText(_ context.Context, text string) (int, error) {
	return len(text), nil
}

func TestTokenCountBatcherDefaultsToPlainTextWithoutReserve(t *testing.T) {
	batcher, err := documentpipeline.NewTokenCountBatcher(documentpipeline.TokenCountBatcherConfig{
		Estimator: textLengthEstimator{},
		MaxTokens: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := document.NewDocument("12345", nil)
	second, _ := document.NewDocument("67890", nil)

	batches, err := batcher.Batch(context.Background(), []*document.Document{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("batches = %#v, want one full 10-token batch", batches)
	}
}

func TestTokenCountBatcherReserveReducesBudget(t *testing.T) {
	batcher, err := documentpipeline.NewTokenCountBatcher(documentpipeline.TokenCountBatcherConfig{
		Estimator: textLengthEstimator{},
		MaxTokens: 10,
		Reserve:   0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := document.NewDocument("12345", nil)
	second, _ := document.NewDocument("67890", nil)

	batches, err := batcher.Batch(context.Background(), []*document.Document{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 {
		t.Fatalf("batch count = %d, want 2 with an 8-token budget", len(batches))
	}
}

func TestTokenCountBatcherValidatesConstructorInput(t *testing.T) {
	tests := []struct {
		name   string
		config documentpipeline.TokenCountBatcherConfig
	}{
		{name: "estimator required"},
		{name: "negative max", config: documentpipeline.TokenCountBatcherConfig{
			Estimator: textLengthEstimator{}, MaxTokens: -1,
		}},
		{name: "invalid reserve", config: documentpipeline.TokenCountBatcherConfig{
			Estimator: textLengthEstimator{}, Reserve: 1,
		}},
		{name: "invalid mode", config: documentpipeline.TokenCountBatcherConfig{
			Estimator: textLengthEstimator{}, Mode: "unknown",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := documentpipeline.NewTokenCountBatcher(test.config); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

type failingEstimator struct{ err error }

func (e failingEstimator) EstimateText(context.Context, string) (int, error) {
	return 0, e.err
}

func TestTokenCountBatcherPropagatesEstimatorError(t *testing.T) {
	want := errors.New("estimate failed")
	batcher, err := documentpipeline.NewTokenCountBatcher(documentpipeline.TokenCountBatcherConfig{
		Estimator: failingEstimator{err: want},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := document.NewDocument("text", nil)
	if _, err := batcher.Batch(context.Background(), []*document.Document{doc}); !errors.Is(err, want) {
		t.Fatalf("Batch error = %v, want %v", err, want)
	}
}
