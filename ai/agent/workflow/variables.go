package workflow

import (
	"sync"
)

type Variable interface {
	Type() string
	Value() any
	Scan(any) error
}

type Variables []Variable

func (variables Variables) Type() string {
	return "variables"
}

func (variables Variables) Value() any {
	values := make([]any, len(variables))
	for _, variable := range variables {
		values = append(values, variable.Value())
	}
	return values
}

func (variables Variables) Scan(value any) error {
	return nil
}

type VariablePool struct {
	store map[string]map[string]Variable
	mu    sync.RWMutex
}

func (v *VariablePool) Set(node Node, key string, value Variable) {
	v.mu.Lock()
	defer v.mu.Unlock()
	nodeID := node.ID()
	nodeStore, ok := v.store[nodeID]
	if !ok {
		nodeStore = make(map[string]Variable)
	}
	nodeStore[key] = value
	v.store[nodeID] = nodeStore
}

func (v *VariablePool) SetMap(node Node, value map[string]Variable) {
	v.mu.Lock()
	defer v.mu.Unlock()
	nodeID := node.ID()
	v.store[nodeID] = value
}
