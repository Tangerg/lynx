package operation

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const RuntimeSubscribe Name = "runtime.subscribe"

func registerRuntimeSubscription(registry *Registry) {
	// Only a subscription that registers watches needs features.fileWatch —
	// subscribing for the global topics is always available (§7.1). The condition
	// treats `watches: []` as "no watches", so an explicitly empty list and an absent
	// one behave alike.
	//
	// A topic this build does not advertise is refused with capability_not_negotiated
	// by the handler, which is the only place that knows the composition's answer.
	registry.Subscription(MethodMeta{
		Name: RuntimeSubscribe,
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "watches", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureFileWatch},
		}},
	}, func(service interface {
		SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error)
	}, ctx context.Context, request protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
		return service.SubscribeRuntime(ctx, request)
	})
}
