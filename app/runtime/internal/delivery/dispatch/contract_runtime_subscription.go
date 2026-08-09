package dispatch

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerRuntimeSubscription(registry *Registry) {
	// Only a subscription that registers watches needs features.fileWatch —
	// subscribing for the global topics is always available (§7.1). The condition
	// treats `watches: []` as "no watches", so an explicitly empty list and an absent
	// one behave alike.
	//
	// A topic this build does not advertise is refused with capability_not_negotiated
	// by the handler, which is the only place that knows the composition's answer.
	Subscription(registry, MethodMeta{
		Name: "runtime.subscribe",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "watches", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureFileWatch},
		}},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
		return router.api.SubscribeRuntime(ctx, request)
	}, runtimeEventFramer)
}
