package workflow

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/flow"
)

const (
	TypeParallel NodeType = "Parallel"
)

type ParallelConfig struct {
	ID               string
	Nodes            []Node
	ContinueOnError  bool
	ConcurrencyLimit int
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

	return nil
}

var _ Node = (*Parallel)(nil)

type Parallel struct {
	config   ParallelConfig
	metadata map[string]any
}

func NewParallel(cfg ParallelConfig) (*Parallel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Parallel{
		config: cfg,
		metadata: map[string]any{
			"id":                cfg.ID,
			"type":              TypeParallel,
			"continue_on_error": cfg.ContinueOnError,
			"concurrency_limit": cfg.ConcurrencyLimit,
			"count":             len(cfg.Nodes),
			"nodes": lo.Map(cfg.Nodes, func(node Node, _ int) map[string]any {
				return node.Metadata()
			}),
		},
	}, nil
}

func (p *Parallel) ID() string {
	return p.config.ID
}

func (p *Parallel) Type() NodeType {
	return TypeParallel
}

func (p *Parallel) Metadata() map[string]any {
	return maps.Clone(p.metadata)
}
func (p *Parallel) Run(ctx context.Context, vars *VariablePool) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		p.run(ctx, vars, yield)
	}
}

func (p *Parallel) run(ctx context.Context, vars *VariablePool, yield func(Event, error) bool) {
	nodes := lo.Map(p.config.Nodes, func(node Node, _ int) func(context.Context, *VariablePool) (*VariablePool, error) {
		return func(ctx context.Context, variables *VariablePool) (*VariablePool, error) {
			for event, err := range node.Run(ctx, variables) {
				if err != nil {
					yield(event, err)
					return variables, err
				}
				if !yield(event, nil) {
					break
				}
			}
			return variables, nil
		}
	})

	parallel, err := flow.
		NewParallelBuilder[*VariablePool, *VariablePool]().
		WithConcurrencyLimit(p.config.ConcurrencyLimit).
		WithContinueOnError(p.config.ContinueOnError).
		WithProcessors(nodes).
		Build()
	if err != nil {
		yield(nil, err)
		return
	}

	_, err = parallel.Run(ctx, vars)
	if err != nil {
		yield(nil, err)
		return
	}
}
