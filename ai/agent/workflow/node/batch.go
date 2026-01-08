package node

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
)

const (
	TypeBatch Type = "Batch"
)

type BatchConfig struct {
	ID               string
	Node             Node
	Segmenter        func(context.Context, workflow.State) ([]any, error)
	Aggregator       func(context.Context, []any) (workflow.State, error)
	ContinueOnError  bool
	ConcurrencyLimit int
}

func (cfg *BatchConfig) svalidate() error {
	if cfg == nil {
		return errors.New("batch config cannot be nil")
	}
	if cfg.ID == "" {
		return errors.New("empty ID")
	}
	if cfg.Node == nil {
		return errors.New("batch node cannot be nil")
	}

	if cfg.Segmenter == nil {
		return errors.New("segmenter is required: batch processing needs a function to divide input into segments")
	}

	if cfg.Aggregator == nil {
		return errors.New("aggregator is required: batch processing needs a function to combine segment results")
	}
	return nil
}

var _ Node = (*Batch)(nil)

type Batch struct {
	id       string
	node     Node
	batch    *flow.Batch[workflow.State, workflow.State, any, any]
	metadata Metadata
}

func NewBatch(cfg *BatchConfig) (*Batch, error) {
	if err := cfg.svalidate(); err != nil {
		return nil, err
	}
	batch, err := flow.NewBatch(&flow.BatchConfig[workflow.State, workflow.State, any, any]{
		Node: flow.Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return cfg.Node.Run(ctx, input.(workflow.State))
		}),
		Segmenter:        cfg.Segmenter,
		Aggregator:       cfg.Aggregator,
		ContinueOnError:  cfg.ContinueOnError,
		ConcurrencyLimit: cfg.ConcurrencyLimit,
	})
	if err != nil {
		return nil, err
	}
	return &Batch{
		id:    cfg.ID,
		node:  cfg.Node,
		batch: batch,
		metadata: Metadata{
			ID:   cfg.ID,
			Type: TypeBatch,
			Value: map[string]any{
				"continue_on_error": cfg.ContinueOnError,
				"concurrency_limit": cfg.ConcurrencyLimit,
			},
		},
	}, nil
}

func (b *Batch) ID() string {
	return b.id
}

func (b *Batch) Type() Type {
	return TypeBatch
}

func (b *Batch) Metadata() Metadata {
	return b.metadata.WithInner(b.node.Metadata())
}

func (b *Batch) Run(ctx context.Context, input workflow.State) (workflow.State, error) {
	return b.batch.Run(ctx, input)
}
