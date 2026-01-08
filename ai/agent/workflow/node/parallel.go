package node

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
)

const (
	TypeParallel Type = "Parallel"
)

type ParallelConfig struct {
	ID                string
	Nodes             []Node
	Aggregator        func(context.Context, []any) (workflow.State, error)
	WaitCount         int
	RequiredSuccesses int
	ContinueOnError   bool
	CancelRemaining   bool
}

func (cfg *ParallelConfig) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.ID == "" {
		return errors.New("empty ID")
	}
	if len(cfg.Nodes) == 0 {
		return errors.New("parallel must contain at least one node: no processing units defined")
	}

	if cfg.Aggregator == nil {
		return errors.New("parallel must have aggregator: function required to combine parallel results")
	}

	return nil
}

var _ Node = (*Parallel)(nil)

type Parallel struct {
	id       string
	nodes    []Node
	parallel *flow.Parallel[workflow.State, workflow.State]
	metadata Metadata
}

func NewParallel(cfg *ParallelConfig) (*Parallel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	parallel, err := flow.NewParallel(&flow.ParallelConfig[workflow.State, workflow.State]{
		Nodes: lo.Map(cfg.Nodes, func(node Node, _ int) flow.Node[workflow.State, any] {
			return flow.Processor[workflow.State, any](func(ctx context.Context, input workflow.State) (any, error) {
				return node.Run(ctx, input)
			})
		}),
		Aggregator:        cfg.Aggregator,
		WaitCount:         cfg.WaitCount,
		RequiredSuccesses: cfg.RequiredSuccesses,
		ContinueOnError:   cfg.ContinueOnError,
		CancelRemaining:   cfg.CancelRemaining,
	})
	if err != nil {
		return nil, err
	}
	return &Parallel{
		id:       cfg.ID,
		nodes:    cfg.Nodes,
		parallel: parallel,
		metadata: Metadata{
			ID:   cfg.ID,
			Type: TypeParallel,
		},
	}, nil
}

func (p *Parallel) ID() string {
	return p.id
}

func (p *Parallel) Type() Type {
	return TypeParallel
}

func (p *Parallel) Metadata() Metadata {
	return p.metadata
}

func (p *Parallel) Run(ctx context.Context, input workflow.State) (workflow.State, error) {
	return p.parallel.Run(ctx, input)
}
