package goap

import "github.com/Tangerg/lynx/agent/core"

type searchNode struct {
	state  core.WorldState
	gScore float64
	fScore float64
}

type openList []*searchNode

func (o openList) Len() int           { return len(o) }
func (o openList) Less(i, j int) bool { return o[i].fScore < o[j].fScore }
func (o openList) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }

func (o *openList) Push(x any) {
	*o = append(*o, x.(*searchNode))
}

func (o *openList) Pop() any {
	old := *o
	last := len(old) - 1
	node := old[last]
	old[last] = nil
	*o = old[:last]
	return node
}

type edge struct {
	prevKey string
	action  core.Action
}
