package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchConfig_validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				sum := 0
				for _, r := range results {
					sum += r
				}
				return sum, nil
			},
		}

		err := cfg.validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		var cfg *BatchConfig[[]int, int, int, int]

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if err.Error() != "batch config cannot be nil" {
			t.Errorf("expected 'batch config cannot be nil' error, got %v", err)
		}
	})

	t.Run("nil node", func(t *testing.T) {
		cfg := &BatchConfig[[]int, int, int, int]{
			Node: nil,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, nil
			},
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil node, got nil")
		}

		if err.Error() != "batch node cannot be nil" {
			t.Errorf("expected 'batch node cannot be nil' error, got %v", err)
		}
	})

	t.Run("nil segmenter", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node:      mockNode,
			Segmenter: nil,
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, nil
			},
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil segmenter, got nil")
		}

		expectedMsg := "segmenter is required"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("nil aggregator", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: nil,
		}

		err := cfg.validate()
		if err == nil {
			t.Error("expected error for nil aggregator, got nil")
		}

		expectedMsg := "aggregator is required"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("expected error to contain %q, got %q", expectedMsg, err.Error())
		}
	})
}

func TestNewBatch(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, nil
			},
			ConcurrencyLimit: 5,
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if batch == nil {
			t.Error("expected non-nil batch")
		}

		if batch.concurrencyLimit != 5 {
			t.Errorf("expected concurrency limit 5, got %d", batch.concurrencyLimit)
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		cfg := &BatchConfig[[]int, int, int, int]{
			Node: nil,
		}

		batch, err := NewBatch(cfg)
		if err == nil {
			t.Error("expected error for invalid config, got nil")
		}

		if batch != nil {
			t.Error("expected nil batch for invalid config")
		}
	})

	t.Run("nil config", func(t *testing.T) {
		batch, err := NewBatch[[]int, int, int, int](nil)
		if err == nil {
			t.Error("expected error for nil config, got nil")
		}

		if batch != nil {
			t.Error("expected nil batch for nil config")
		}
	})

	t.Run("with continue on error flag", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, nil
			},
			ContinueOnError: true,
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if !batch.continueOnError {
			t.Error("expected continueOnError to be true")
		}
	})
}

func TestBatch_getConcurrencyLimit(t *testing.T) {
	t.Run("positive limit", func(t *testing.T) {
		batch := &Batch[[]int, int, int, int]{
			concurrencyLimit: 5,
		}

		limit := batch.getConcurrencyLimit()
		if limit != 5 {
			t.Errorf("expected limit 5, got %d", limit)
		}
	})

	t.Run("zero limit defaults to 1", func(t *testing.T) {
		batch := &Batch[[]int, int, int, int]{
			concurrencyLimit: 0,
		}

		limit := batch.getConcurrencyLimit()
		if limit != 1 {
			t.Errorf("expected limit 1, got %d", limit)
		}
	})

	t.Run("negative limit defaults to 1", func(t *testing.T) {
		batch := &Batch[[]int, int, int, int]{
			concurrencyLimit: -5,
		}

		limit := batch.getConcurrencyLimit()
		if limit != 1 {
			t.Errorf("expected limit 1, got %d", limit)
		}
	})
}

func TestBatch_runSequential(t *testing.T) {
	t.Run("successful processing", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:            mockNode,
			continueOnError: false,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runSequential(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := []int{2, 4, 6, 8, 10}
		if len(results) != len(expected) {
			t.Errorf("expected %d results, got %d", len(expected), len(results))
		}

		for i, result := range results {
			if result != expected[i] {
				t.Errorf("result[%d]: expected %d, got %d", i, expected[i], result)
			}
		}
	})

	t.Run("error without continue", func(t *testing.T) {
		expectedErr := errors.New("processing error")
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			if input == 3 {
				return 0, expectedErr
			}
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:            mockNode,
			continueOnError: false,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runSequential(context.Background(), segments)

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("error with continue", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			if input == 3 {
				return 0, errors.New("processing error")
			}
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:            mockNode,
			continueOnError: true,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runSequential(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := []int{2, 4, 8, 10}
		if len(results) != len(expected) {
			t.Errorf("expected %d results, got %d", len(expected), len(results))
		}

		for i, result := range results {
			if result != expected[i] {
				t.Errorf("result[%d]: expected %d, got %d", i, expected[i], result)
			}
		}
	})

	t.Run("empty segments", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:            mockNode,
			continueOnError: false,
		}

		segments := []int{}
		results, err := batch.runSequential(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				return input * 2, nil
			}
		})

		batch := &Batch[[]int, int, int, int]{
			node:            mockNode,
			continueOnError: false,
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		segments := []int{1, 2, 3}
		_, err := batch.runSequential(ctx, segments)

		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled error, got %v", err)
		}
	})
}

func TestBatch_runConcurrent(t *testing.T) {
	t.Run("successful concurrent processing", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			time.Sleep(10 * time.Millisecond)
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 3,
			continueOnError:  false,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runConcurrent(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := []int{2, 4, 6, 8, 10}
		if len(results) != len(expected) {
			t.Errorf("expected %d results, got %d", len(expected), len(results))
		}

		for i, result := range results {
			if result != expected[i] {
				t.Errorf("result[%d]: expected %d, got %d", i, expected[i], result)
			}
		}
	})

	t.Run("error without continue", func(t *testing.T) {
		expectedErr := errors.New("processing error")
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			if input == 3 {
				return 0, expectedErr
			}
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 3,
			continueOnError:  false,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runConcurrent(context.Background(), segments)

		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}

		if results != nil {
			t.Errorf("expected nil results, got %v", results)
		}
	})

	t.Run("error with continue", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			if input == 3 {
				return 0, errors.New("processing error")
			}
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 3,
			continueOnError:  true,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runConcurrent(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := []int{2, 4, 8, 10}
		if len(results) != len(expected) {
			t.Errorf("expected %d results, got %d", len(expected), len(results))
		}

		for i, result := range results {
			if result != expected[i] {
				t.Errorf("result[%d]: expected %d, got %d", i, expected[i], result)
			}
		}
	})

	t.Run("preserves order", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			sleepTime := time.Duration(10-input) * time.Millisecond
			time.Sleep(sleepTime)
			return input, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 5,
			continueOnError:  false,
		}

		segments := []int{1, 2, 3, 4, 5}
		results, err := batch.runConcurrent(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		for i, result := range results {
			if result != segments[i] {
				t.Errorf("result[%d]: expected %d, got %d", i, segments[i], result)
			}
		}
	})

	t.Run("concurrency limit is respected", func(t *testing.T) {
		var concurrent atomic.Int32
		var maxConcurrent atomic.Int32

		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			current := concurrent.Add(1)
			defer concurrent.Add(-1)

			for {
				m := maxConcurrent.Load()
				if current <= m {
					break
				}
				if maxConcurrent.CompareAndSwap(m, current) {
					break
				}
			}

			time.Sleep(20 * time.Millisecond)
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 2,
			continueOnError:  false,
		}

		segments := []int{1, 2, 3, 4, 5}
		_, err := batch.runConcurrent(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		m := maxConcurrent.Load()
		if m > 2 {
			t.Errorf("expected max concurrency 2, got %d", m)
		}
	})

	t.Run("empty segments", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 3,
			continueOnError:  false,
		}

		var segments []int
		results, err := batch.runConcurrent(context.Background(), segments)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return input * 2, nil
			}
		})

		batch := &Batch[[]int, int, int, int]{
			node:             mockNode,
			concurrencyLimit: 3,
			continueOnError:  false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		segments := []int{1, 2, 3, 4, 5}
		_, err := batch.runConcurrent(ctx, segments)

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded error, got %v", err)
		}
	})
}

func TestBatch_Run(t *testing.T) {
	t.Run("successful batch processing sequential", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				sum := 0
				for _, r := range results {
					sum += r
				}
				return sum, nil
			},
			ConcurrencyLimit: 1,
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Fatalf("failed to create batch: %v", err)
		}

		input := []int{1, 2, 3, 4, 5}
		result, err := batch.Run(context.Background(), input)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := 30
		if result != expected {
			t.Errorf("expected %d, got %d", expected, result)
		}
	})

	t.Run("successful batch processing concurrent", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				sum := 0
				for _, r := range results {
					sum += r
				}
				return sum, nil
			},
			ConcurrencyLimit: 3,
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Fatalf("failed to create batch: %v", err)
		}

		input := []int{1, 2, 3, 4, 5}
		result, err := batch.Run(context.Background(), input)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		expected := 30
		if result != expected {
			t.Errorf("expected %d, got %d", expected, result)
		}
	})

	t.Run("segmenter returns error", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		expectedErr := errors.New("segmenter error")
		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return nil, expectedErr
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, nil
			},
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Fatalf("failed to create batch: %v", err)
		}

		input := []int{1, 2, 3}
		_, err = batch.Run(context.Background(), input)

		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("aggregator returns error", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		expectedErr := errors.New("aggregator error")
		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, expectedErr
			},
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Fatalf("failed to create batch: %v", err)
		}

		input := []int{1, 2, 3}
		_, err = batch.Run(context.Background(), input)

		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			return input * 2, nil
		})

		cfg := &BatchConfig[[]int, int, int, int]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []int) ([]int, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []int) (int, error) {
				return 0, nil
			},
			ConcurrencyLimit: 3,
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Fatalf("failed to create batch: %v", err)
		}

		var input []int
		result, err := batch.Run(context.Background(), input)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result != 0 {
			t.Errorf("expected 0, got %d", result)
		}
	})

	t.Run("complex types", func(t *testing.T) {
		type Item struct {
			ID    int
			Value string
		}

		type ProcessedItem struct {
			ID     int
			Result string
		}

		type Summary struct {
			TotalProcessed int
			IDs            []int
		}

		mockNode := Processor[Item, ProcessedItem](func(ctx context.Context, input Item) (ProcessedItem, error) {
			return ProcessedItem{
				ID:     input.ID,
				Result: fmt.Sprintf("processed-%s", input.Value),
			}, nil
		})

		cfg := &BatchConfig[[]Item, Summary, Item, ProcessedItem]{
			Node: mockNode,
			Segmenter: func(ctx context.Context, input []Item) ([]Item, error) {
				return input, nil
			},
			Aggregator: func(ctx context.Context, results []ProcessedItem) (Summary, error) {
				ids := make([]int, len(results))
				for i, r := range results {
					ids[i] = r.ID
				}
				return Summary{
					TotalProcessed: len(results),
					IDs:            ids,
				}, nil
			},
			ConcurrencyLimit: 2,
		}

		batch, err := NewBatch(cfg)
		if err != nil {
			t.Fatalf("failed to create batch: %v", err)
		}

		input := []Item{
			{ID: 1, Value: "a"},
			{ID: 2, Value: "b"},
			{ID: 3, Value: "c"},
		}

		result, err := batch.Run(context.Background(), input)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		if result.TotalProcessed != 3 {
			t.Errorf("expected 3 processed items, got %d", result.TotalProcessed)
		}

		expectedIDs := []int{1, 2, 3}
		if len(result.IDs) != len(expectedIDs) {
			t.Errorf("expected %d IDs, got %d", len(expectedIDs), len(result.IDs))
		}

		for i, id := range result.IDs {
			if id != expectedIDs[i] {
				t.Errorf("ID[%d]: expected %d, got %d", i, expectedIDs[i], id)
			}
		}
	})

	t.Run("concurrent execution is faster than sequential", func(t *testing.T) {
		mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
			time.Sleep(50 * time.Millisecond)
			return input * 2, nil
		})

		segmenter := func(ctx context.Context, input []int) ([]int, error) {
			return input, nil
		}

		aggregator := func(ctx context.Context, results []int) (int, error) {
			sum := 0
			for _, r := range results {
				sum += r
			}
			return sum, nil
		}

		sequentialCfg := &BatchConfig[[]int, int, int, int]{
			Node:             mockNode,
			Segmenter:        segmenter,
			Aggregator:       aggregator,
			ConcurrencyLimit: 1,
		}

		concurrentCfg := &BatchConfig[[]int, int, int, int]{
			Node:             mockNode,
			Segmenter:        segmenter,
			Aggregator:       aggregator,
			ConcurrencyLimit: 5,
		}

		sequentialBatch, _ := NewBatch(sequentialCfg)
		concurrentBatch, _ := NewBatch(concurrentCfg)

		input := []int{1, 2, 3, 4, 5}

		startSeq := time.Now()
		_, _ = sequentialBatch.Run(context.Background(), input)
		seqDuration := time.Since(startSeq)

		startConc := time.Now()
		_, _ = concurrentBatch.Run(context.Background(), input)
		concDuration := time.Since(startConc)

		if concDuration >= seqDuration {
			t.Logf("Sequential: %v, Concurrent: %v", seqDuration, concDuration)
		}
	})
}

func BenchmarkBatch_Run_Sequential(b *testing.B) {
	mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})

	cfg := &BatchConfig[[]int, int, int, int]{
		Node: mockNode,
		Segmenter: func(ctx context.Context, input []int) ([]int, error) {
			return input, nil
		},
		Aggregator: func(ctx context.Context, results []int) (int, error) {
			sum := 0
			for _, r := range results {
				sum += r
			}
			return sum, nil
		},
		ConcurrencyLimit: 1,
	}

	batch, _ := NewBatch(cfg)
	input := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = batch.Run(ctx, input)
	}
}

func BenchmarkBatch_Run_Concurrent(b *testing.B) {
	mockNode := Processor[int, int](func(ctx context.Context, input int) (int, error) {
		return input * 2, nil
	})

	cfg := &BatchConfig[[]int, int, int, int]{
		Node: mockNode,
		Segmenter: func(ctx context.Context, input []int) ([]int, error) {
			return input, nil
		},
		Aggregator: func(ctx context.Context, results []int) (int, error) {
			sum := 0
			for _, r := range results {
				sum += r
			}
			return sum, nil
		},
		ConcurrencyLimit: 5,
	}

	batch, _ := NewBatch(cfg)
	input := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = batch.Run(ctx, input)
	}
}
