package workflow

import (
	"context"
	"iter"
)

type NodeType string

func (n NodeType) String() string {
	return string(n)
}

type Node interface {
	ID() string
	Type() NodeType
	Metadata() map[string]any
	Run(ctx context.Context, vars *VariablePool) iter.Seq2[Event, error]
}
