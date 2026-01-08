package node

import (
	"github.com/Tangerg/lynx/ai/agent/workflow"
	"github.com/Tangerg/lynx/flow"
)

type Type string

func (n Type) String() string {
	return string(n)
}

type Metadata struct {
	ID string `json:"id"`

	Type Type `json:"type"`

	Value any `json:"value,omitempty"`

	Inner *Metadata `json:"inner,omitempty"`
}

func (m Metadata) WithInner(inner Metadata) Metadata {
	m.Inner = &inner
	return m
}

func MetadataValue[T any](m Metadata) (T, bool) {
	if m.Value == nil {
		var zero T
		return zero, false
	}
	val, ok := m.Value.(T)
	return val, ok
}

type Node interface {
	flow.Node[workflow.State, workflow.State]
	ID() string
	Type() Type
	Metadata() Metadata
}
