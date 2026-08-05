package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerInterrupts(r *Registry) {
	// interrupts.list is its own root because a waiting set belongs to a run TREE,
	// not to one run: it was runs.listOpenInterrupts, which read as "one run's
	// interrupts" and made the aggregate look like a per-run detail.
	//
	// run_not_root is declared because the filter can name a child, and that is a
	// different answer from "nothing is waiting".
	Query(r, MethodMeta{
		Name: "interrupts.list",
		Errors: []string{
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
		return d.api.ListInterrupts(ctx, in)
	})
}
