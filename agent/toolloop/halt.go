package toolloop

import "github.com/Tangerg/lynx/core/model"

// Halt is the control-flow contract a tool error can carry.
//
// It is an alias to [model.Halt] so consumers can use a single
// low-level abstraction regardless of stack layer.
type Halt = model.Halt
