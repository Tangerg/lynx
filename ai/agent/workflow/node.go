package workflow

import (
	"github.com/Tangerg/lynx/flow"
)

type Node interface {
	Name() string
	flow.Node[State, State]
}
