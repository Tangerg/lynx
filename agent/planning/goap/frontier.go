package goap

import "github.com/Tangerg/lynx/agent/core"

type searchNode struct {
	state core.WorldState
	cost  float64
	order uint64
}

type frontier []*searchNode

func (f frontier) Len() int { return len(f) }

func (f frontier) Less(i, j int) bool {
	if f[i].cost != f[j].cost {
		return f[i].cost < f[j].cost
	}
	return f[i].order < f[j].order
}

func (f frontier) Swap(i, j int) { f[i], f[j] = f[j], f[i] }

func (f *frontier) Push(value any) {
	*f = append(*f, value.(*searchNode))
}

func (f *frontier) Pop() any {
	old := *f
	last := len(old) - 1
	node := old[last]
	old[last] = nil
	*f = old[:last]
	return node
}
