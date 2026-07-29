package sessions

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// TodoBoundary is a task list recovered from a run boundary: the value, and
// whether that boundary recorded one at all. The difference is not cosmetic — an
// unrecorded boundary must leave the live list untouched, while a recorded empty
// one must clear it, and a single nil slice cannot say which is meant.
type TodoBoundary struct {
	Items    []todo.Item
	Recorded bool
}

// todoBoundary resolves the task list the boundary at runID held. An empty runID
// is a boundary that keeps no run at all: it predates every list this session ever
// wrote, so its value is the empty list — known, not unknown. Otherwise the answer
// is whatever that run recorded when it ended, including "nothing was recorded",
// which the caller must not turn into emptiness (an imported run's boundaries were
// never captured; see [TodoBoundaries]).
func (c *Coordinator) todoBoundary(ctx context.Context, runID string) (TodoBoundary, error) {
	if runID == "" {
		return TodoBoundary{Recorded: true}, nil
	}
	if c.boundaries == nil {
		return TodoBoundary{}, nil
	}
	items, recorded, err := c.boundaries.Boundary(ctx, runID)
	if err != nil {
		return TodoBoundary{}, err
	}
	return TodoBoundary{Items: items, Recorded: recorded}, nil
}
