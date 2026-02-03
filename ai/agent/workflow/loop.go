package workflow

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/Tangerg/lynx/flow"
)

const (
	TypeLoop NodeType = "Loop"
)

type LoopConfig struct {
	ID            string
	Node          Node
	MaxIterations int
	Terminator    func(context.Context, int, *VariablePool) bool
}

func (cfg *LoopConfig) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.ID == "" {
		return errors.New("empty ID")
	}
	if cfg.MaxIterations < 0 {
		return errors.New("invalid MaxIterations")
	}
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 10
	}
	if cfg.Terminator == nil {
		cfg.Terminator = func(context.Context, int, *VariablePool) bool { return true }
	}
	return nil
}

var _ Node = (*Loop)(nil)

type Loop struct {
	config   LoopConfig
	metadata map[string]any
}

func NewLoop(cfg LoopConfig) (*Loop, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Loop{
		config: cfg,
		metadata: map[string]any{
			"id":             cfg.ID,
			"type":           TypeLoop,
			"max_iterations": cfg.MaxIterations,
			"node":           cfg.Node.Metadata(),
		},
	}, nil
}

func (l *Loop) ID() string {
	return l.config.ID
}

func (l *Loop) Type() NodeType {
	return TypeLoop
}

func (l *Loop) Metadata() map[string]any {
	return maps.Clone(l.metadata)
}

func (l *Loop) Run(ctx context.Context, vars *VariablePool) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		l.run(ctx, vars, yield)
	}
}

func (l *Loop) run(ctx context.Context, vars *VariablePool, yield func(Event, error) bool) {
	loop, err := flow.
		NewLoopBuilder[*VariablePool]().
		WithMaxIterations(l.config.MaxIterations).
		WithProcessor(
			func(ctx context.Context, iteration int, variables *VariablePool) (*VariablePool, bool, error) {
				for event, err := range l.config.Node.Run(ctx, variables) {
					if err != nil {
						yield(nil, err)
						return variables, true, err
					}
					if !yield(event, nil) {
						return variables, true, nil
					}
				}
				return variables, l.config.Terminator(ctx, iteration, variables), nil
			},
		).
		Build()
	if err != nil {
		yield(nil, err)
		return
	}

	_, err = loop.Run(ctx, vars)
	if err != nil {
		yield(nil, err)
		return
	}
}
