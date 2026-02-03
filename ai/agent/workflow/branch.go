package workflow

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/Tangerg/lynx/flow"
	"github.com/samber/lo"
)

const (
	TypeBranch = "Branch"
)

type BranchConfig struct {
	ID             string
	Branches       map[string]Node
	BranchResolver func(context.Context, *VariablePool) string
}

func (cfg *BranchConfig) validate() error {
	if cfg == nil {
		return errors.New("nil config")
	}
	if cfg.ID == "" {
		return errors.New("empty ID")
	}
	if len(cfg.Branches) == 0 {
		return errors.New("empty branches")
	}
	if cfg.BranchResolver == nil && len(cfg.Branches) > 1 {
		return errors.New("empty branch resolver")
	}
	return nil
}

var _ Node = (*Branch)(nil)

type Branch struct {
	config   BranchConfig
	metadata map[string]any
}

func NewBranch(cfg BranchConfig) (*Branch, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &Branch{
		config: cfg,
		metadata: map[string]any{
			"id":   cfg.ID,
			"type": TypeBranch,
			"branches": lo.MapValues(cfg.Branches, func(node Node, branch string) map[string]any {
				return map[string]any{
					"branch": branch,
					"node":   node.Metadata(),
				}
			}),
		},
	}, nil
}

func (b *Branch) ID() string {
	return b.config.ID
}

func (b *Branch) Type() NodeType {
	return TypeBranch
}

func (b *Branch) Metadata() map[string]any {
	return maps.Clone(b.metadata)
}

func (b *Branch) Run(ctx context.Context, vars *VariablePool) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		b.run(ctx, vars, yield)
	}
}

func (b *Branch) run(ctx context.Context, vars *VariablePool, yield func(Event, error) bool) {
	branchs := lo.MapEntries(
		b.config.Branches,
		func(branch string, node Node) (string, func(context.Context, *VariablePool) (*VariablePool, error)) {
			return branch, func(ctx context.Context, variables *VariablePool) (*VariablePool, error) {
				for event, err := range node.Run(ctx, variables) {
					if err != nil {
						yield(nil, err)
						return variables, err
					}
					if !yield(event, nil) {
						return variables, nil
					}
				}
				return variables, nil
			}
		})

	branch, err := flow.
		NewBranchBuilder[*VariablePool, *VariablePool]().
		WithBranchResolver(b.config.BranchResolver).
		WithBranches(branchs).
		Build()
	if err != nil {
		yield(nil, err)
		return
	}

	_, err = branch.Run(ctx, vars)
	if err != nil {
		yield(nil, err)
		return
	}

}
