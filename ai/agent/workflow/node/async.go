package node

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
	"github.com/Tangerg/lynx/pkg/sync"
)

const (
	TypeAsync Type = "Async"
)

type AsyncConfig struct {
	ID   string
	Node Node
	Pool sync.Pool
}

func (cfg *AsyncConfig) validate() error {
	if cfg == nil {
		return errors.New("async config cannot be nil")
	}

	if cfg.Node == nil {
		return errors.New("async node cannot be nil")
	}
	return nil
}

var _ Node = (*Async)(nil)

type Async struct {
	id       string
	node     Node
	async    *flow.Async[workflow.State, any]
	metadata Metadata
}

func NewAsync(cfg *AsyncConfig) (*Async, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	async, err := flow.NewAsync(&flow.AsyncConfig[workflow.State, any]{
		Node: flow.Processor[workflow.State, any](func(ctx context.Context, input workflow.State) (any, error) {
			return cfg.Node.Run(ctx, input)
		}),
		Pool: cfg.Pool,
	})
	if err != nil {
		return nil, err
	}
	return &Async{
		id:    cfg.ID,
		node:  cfg.Node,
		async: async,
		metadata: Metadata{
			ID:   cfg.ID,
			Type: TypeAsync,
			Value: map[string]any{
				"start_at": time.Now(),
			},
		},
	}, nil
}

func (a *Async) ID() string {
	return a.id
}

func (a *Async) Type() Type {
	return TypeAsync
}

func (a *Async) Metadata() Metadata {
	return a.metadata.WithInner(a.node.Metadata())
}

func (a *Async) Run(ctx context.Context, input workflow.State) (workflow.State, error) {
	future, err := a.async.RunType(ctx, input)
	if err != nil {
		return input, err
	}
	input.Futures = append(input.Futures, future)
	return input, nil
}
