package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerInterrupts(registry *Registry) {
	// interrupts.list is its own root because a waiting set belongs to a run TREE,
	// not to one run: it was runs.listOpenInterrupts, which read as "one run's
	// interrupts" and made the aggregate look like a per-run detail.
	//
	// run_not_root is declared because the filter can name a child, and that is a
	// different answer from "nothing is waiting".
	Query(registry, MethodMeta{
		Name: "interrupts.list",
		Errors: []string{
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(service interface {
		ListInterrupts(context.Context, protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error)
	}, ctx context.Context, request protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
		return service.ListInterrupts(ctx, request)
	})
}
