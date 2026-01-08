package node

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
)

const (
	TypeLoop Type = "Loop"
)

type LoopConfig struct {
	ID            string
	Node          Node
	MaxIterations int
	Terminator    func(context.Context, int, workflow.State, workflow.State) (bool, error)
}

func (cfg *LoopConfig) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.ID == "" {
		return errors.New("empty ID")
	}
	// other field will be validated in flow.NewLoop
	return nil
}

var _ Node = (*Loop)(nil)

type Loop struct {
	id       string
	node     Node
	loop     *flow.Loop[workflow.State, workflow.State]
	metadata Metadata
}

func NewLoop(cfg *LoopConfig) (*Loop, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	loop, err := flow.NewLoop(&flow.LoopConfig[workflow.State, workflow.State]{
		Node:          cfg.Node,
		MaxIterations: cfg.MaxIterations,
		Terminator:    cfg.Terminator,
	})
	if err != nil {
		return nil, err
	}
	return &Loop{
		id:   cfg.ID,
		node: cfg.Node,
		loop: loop,
		metadata: Metadata{
			ID:   cfg.ID,
			Type: TypeLoop,
			Value: map[string]any{
				"max_iterations": cfg.MaxIterations,
			},
		},
	}, nil
}

func (l *Loop) ID() string {
	return l.id
}

func (l *Loop) Type() Type {
	return TypeLoop
}

func (l *Loop) Metadata() Metadata {
	return l.metadata.WithInner(l.node.Metadata())
}

func (l *Loop) Run(ctx context.Context, input workflow.State) (workflow.State, error) {
	return l.loop.Run(ctx, input)
}
