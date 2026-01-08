package node

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
)

const (
	TypeBranch = "Branch"
)

type BranchConfig struct {
	ID             string
	Node           Node
	Branches       map[string]Node
	BranchResolver func(context.Context, workflow.State, workflow.State) (string, error)
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
	if cfg.BranchResolver == nil {
		return errors.New("empty branch resolver")
	}
	return nil
}

var _ Node = (*Branch)(nil)

type Branch struct {
	id       string
	node     Node
	branch   *flow.Branch
	metadata Metadata
}

func NewBranch(cfg *BranchConfig) (*Branch, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	branch, err := flow.NewBranch(&flow.BranchConfig{
		Node: flow.Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return cfg.Node.Run(ctx, input.(workflow.State))
		}),
		Branches: lo.MapEntries(cfg.Branches, func(branch string, node Node) (string, flow.Node[any, any]) {
			return branch, flow.Processor[any, any](func(ctx context.Context, input any) (any, error) {
				return cfg.Node.Run(ctx, input.(workflow.State))
			})
		}),
		BranchResolver: func(ctx context.Context, input any, output any) (string, error) {
			return cfg.BranchResolver(ctx, input.(workflow.State), output.(workflow.State))
		},
	})
	if err != nil {
		return nil, err
	}

	return &Branch{
		id:     cfg.ID,
		node:   cfg.Node,
		branch: branch,
		metadata: Metadata{
			ID:   cfg.ID,
			Type: TypeBranch,
			Value: map[string]any{
				"branches": lo.MapEntries(cfg.Branches, func(branch string, node Node) (string, map[string]any) {
					return branch, map[string]any{
						"id":       node.ID(),
						"type":     node.Type(),
						"metadata": node.Metadata(),
					}
				}),
			},
		},
	}, nil
}

func (b *Branch) ID() string {
	return b.id
}

func (b *Branch) Type() Type {
	return TypeBranch
}

func (b *Branch) Metadata() Metadata {
	return b.metadata.WithInner(b.node.Metadata())
}

func (b *Branch) Run(ctx context.Context, input workflow.State) (workflow.State, error) {
	output, err := b.branch.Run(ctx, input)
	if err != nil {
		return input, err
	}
	return output.(workflow.State), nil
}
