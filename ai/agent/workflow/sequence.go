package workflow

import (
	"context"
	"fmt"
	"iter"
	"maps"

	"github.com/samber/lo"
)

const (
	TypeSequence NodeType = "Sequence"
)

var _ Node = (*Sequence)(nil)

type Sequence struct {
	id       string
	nodes    []Node
	metadata map[string]any
}

func NewSequence(id string, nodes ...Node) (*Sequence, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("sequence node must have at least one node")
	}
	return &Sequence{
		id:    id,
		nodes: nodes,
		metadata: map[string]any{
			"id":    id,
			"type":  TypeSequence,
			"count": len(nodes),
			"nodes": lo.Map(nodes, func(node Node, _ int) map[string]any {
				return node.Metadata()
			}),
		},
	}, nil
}

func (f *Sequence) ID() string {
	return f.id
}

func (f *Sequence) Type() NodeType {
	return TypeSequence
}

func (f *Sequence) Metadata() map[string]any {
	return maps.Clone(f.metadata)
}

func (f *Sequence) Run(ctx context.Context, vars *VariablePool) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for _, node := range f.nodes {
			for event, err := range node.Run(ctx, vars) {
				if err != nil {
					yield(nil, err)
					return
				}
				if !yield(event, nil) {
					return
				}
			}
		}
	}
}
