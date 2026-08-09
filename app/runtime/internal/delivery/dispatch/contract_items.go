package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerItems(registry *Registry) {
	// Either scope can name something that does not exist, and the client's next move
	// differs — find the session, or find the run — so both refusals are declared.
	// Asking a run scope for its subtree needs features.subagents; the scope itself
	// does not, since a root run is a run.
	Query(registry, MethodMeta{
		Name: "items.list",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
		},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "scope.includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
		return router.api.ListItems(ctx, request)
	})
}
