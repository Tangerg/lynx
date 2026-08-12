package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type changeBinding interface {
	SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest, embedded.SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error)
}

func (r *Runtime) Supports(topic changefeed.Topic) bool {
	return r.profile.SupportsRuntimeTopic(string(topic))
}

func (r *Runtime) Subscribe(ctx context.Context, subscription changefeed.Subscription) (changefeed.EventStream, error) {
	if err := subscription.Validate(); err != nil {
		return nil, err
	}
	if len(subscription.Watches) != 0 {
		if err := r.requireFeature(runtimeprofile.FeatureFileWatch); err != nil {
			return nil, err
		}
	}
	limits := r.profile.Limits.RuntimeSubscription
	if limits.MaxTopics > 0 && len(subscription.Topics) > limits.MaxTopics {
		return nil, fmt.Errorf("runtime change subscription has %d topics; maximum is %d", len(subscription.Topics), limits.MaxTopics)
	}
	if limits.MaxWatches > 0 && len(subscription.Watches) > limits.MaxWatches {
		return nil, fmt.Errorf("runtime change subscription has %d watches; maximum is %d", len(subscription.Watches), limits.MaxWatches)
	}
	for _, topic := range subscription.Topics {
		if !r.Supports(topic) {
			return nil, errors.New("runtime does not support change topic " + string(topic))
		}
	}
	wire := protocol.RuntimeSubscribeRequest{
		Topics:  make([]protocol.RuntimeTopic, 0, len(subscription.Topics)),
		Watches: make([]protocol.WatchSpec, 0, len(subscription.Watches)),
	}
	for _, topic := range subscription.Topics {
		wire.Topics = append(wire.Topics, protocol.RuntimeTopic(topic))
	}
	for _, watch := range subscription.Watches {
		wire.Watches = append(wire.Watches, protocol.WatchSpec{
			WatchID: watch.ID, Workspace: protocol.WorkspaceRef{Path: watch.Workspace},
		})
	}
	ack, events, err := r.changes.SubscribeRuntime(ctx, wire, r.changeSubscriptionOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if ack == nil || events == nil {
		return nil, runtimeContractViolation("subscribe runtime changes returned an incomplete stream")
	}
	return func(yield func(changefeed.Event, error) bool) {
		for event, err := range events {
			if err != nil {
				yield(changefeed.Event{}, classifyError(err))
				return
			}
			if err := protocol.ValidateWireTree(event); err != nil {
				yield(changefeed.Event{}, runtimeContractViolation("runtime change event is invalid: %v", err))
				return
			}
			projected := projectRuntimeEvent(event)
			if err := subscription.ValidateEvent(projected); err != nil {
				yield(changefeed.Event{}, runtimeContractViolation("runtime change event cannot be projected: %v", err))
				return
			}
			if !yield(projected, nil) {
				return
			}
		}
	}, nil
}

func projectRuntimeEvent(event protocol.RuntimeEvent) changefeed.Event {
	projected := changefeed.Event{
		Type: changefeed.EventType(event.Type), Sequence: event.Sequence, WatchID: event.WatchID,
		Paths: slices.Clone(event.Paths), Names: slices.Clone(event.Names), ServerIDs: slices.Clone(event.ServerIDs),
		ScheduleIDs: slices.Clone(event.ScheduleIDs), SessionIDs: slices.Clone(event.SessionIDs),
		RunIDs: slices.Clone(event.RunIDs), StateKey: changefeed.StateKey(event.Key), WatchIDs: slices.Clone(event.WatchIDs),
	}
	if event.Workspace != nil {
		projected.Workspace = event.Workspace.Path
	}
	projected.Topics = make([]changefeed.Topic, 0, len(event.Topics))
	for _, topic := range event.Topics {
		projected.Topics = append(projected.Topics, changefeed.Topic(topic))
	}
	return projected
}
