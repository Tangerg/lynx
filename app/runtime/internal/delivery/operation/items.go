package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
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
	}, func(service interface {
		ListItems(context.Context, protocol.ListItemsRequest) (*protocol.ListItemsResponse, error)
	}, ctx context.Context, request protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
		return service.ListItems(ctx, request)
	})
}
