package node

import (
	"context"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
)

const (
	TypeFlow Type = "flow"
)

var _ Node = (*Flow)(nil)

type Flow struct {
	id       string
	nodes    []Node
	flow     *flow.Flow
	metadata Metadata
}

func NewFlow(id string, nodes ...Node) (*Flow, error) {
	newFlow, err := flow.NewFlow(lo.Map(nodes, func(node Node, _ int) flow.Node[any, any] {
		return flow.Processor[any, any](func(ctx context.Context, input any) (any, error) {
			return node.Run(ctx, input.(workflow.State))
		})
	})...)
	if err != nil {
		return nil, err
	}

	return &Flow{
		id:    id,
		nodes: nodes,
		flow:  newFlow,
		metadata: Metadata{
			ID:   id,
			Type: TypeFlow,
			Value: lo.Map(nodes, func(node Node, _ int) map[string]any {
				return map[string]any{
					"id":       node.ID(),
					"type":     node.Type(),
					"metadata": node.Metadata(),
				}
			}),
		},
	}, nil
}

func (f *Flow) ID() string {
	return f.id
}

func (f *Flow) Type() Type {
	return TypeFlow
}

func (f *Flow) Metadata() Metadata {
	return f.metadata
}

func (f *Flow) Run(ctx context.Context, input workflow.State) (workflow.State, error) {
	output, err := f.flow.Run(ctx, input)
	if err != nil {
		return input, err
	}
	return output.(workflow.State), nil
}
