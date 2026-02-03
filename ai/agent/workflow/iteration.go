package workflow

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/Tangerg/lynx/flow"
)

const (
	TypeIteration NodeType = "Iteration"
)

type IterationConfig struct {
	ID               string
	Processor        func(ctx context.Context, index int, item Variable) (Variable, error)
	ContinueOnError  bool
	ConcurrencyLimit int
}

func (cfg *IterationConfig) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.ID == "" {
		return errors.New("empty ID")
	}
	if cfg.Processor == nil {
		return errors.New("nil processor")
	}
	return nil
}

// Iteration TODO use more graceful handle vars
type Iteration struct {
	config   IterationConfig
	metadata map[string]any
}

func NewIteration(cfg IterationConfig) (*Iteration, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Iteration{
		config: cfg,
		metadata: map[string]any{
			"id":                cfg.ID,
			"type":              TypeIteration,
			"concurrency_limit": cfg.ConcurrencyLimit,
			"continue_on_error": cfg.ContinueOnError,
		},
	}, nil
}

func (i *Iteration) ID() string {
	return i.config.ID
}

func (i *Iteration) Type() NodeType {
	return TypeIteration
}

func (i *Iteration) Metadata() map[string]any {
	return maps.Clone(i.metadata)
}

func (i *Iteration) Run(ctx context.Context, vars *VariablePool) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		i.run(ctx, vars, yield)
	}
}

func (i *Iteration) run(ctx context.Context, vars *VariablePool, yield func(Event, error) bool) {
	iteration, err := flow.
		NewIterationBuilder[Variable, Variable]().
		WithConcurrencyLimit(i.config.ConcurrencyLimit).
		WithContinueOnError(i.config.ContinueOnError).
		WithProcessor(i.config.Processor).
		Build()
	if err != nil {
		yield(nil, err)
		return
	}
	outputs, err := iteration.Run(ctx, []Variable{})
	if err != nil {
		yield(nil, err)
		return
	}
	var varArr Variables
	for _, value := range outputs {
		varArr = append(varArr, value.Value)
	}

	vars.Set(i, "output", varArr)
}
