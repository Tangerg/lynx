package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"sync"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// errSubscriptionAdmissionsClosed reports that a runtime subscription could not
// start because the Server has stopped admitting new streams.
var errSubscriptionAdmissionsClosed = errors.New("server: runtime subscriptions are closed")

// workspaceHub fans runtime change signals out to the live runtime.subscribe
// streams (§7.1). It is the non-run, ephemeral counterpart to the per-run hubs:
// coalescing rather than back-pressuring the publisher (a change signal carries no
// truth of its own, so several of them collapse into "re-read these topics"),
// connection-scoped (no retained replay), shared by the whole app.
//
// It never drops an invalidation and never skips a sequence number. A slow
// subscriber's undelivered signals fold into one pending resync naming exactly the
// topics involved, which goes out as soon as its queue has room — either behind the
// next signal or when the consumer drains one. The two are not interchangeable: a
// gap tells a client only that something was lost, while a resync tells it what to
// read, and inferring loss from a missing number means a client that never notices
// on a quiet stream.
type workspaceHub struct {
	mu            sync.Mutex
	subscriptions map[*workspaceSubscription]struct{}
	closed        bool
}

func newWorkspaceHub() *workspaceHub {
	return &workspaceHub{subscriptions: make(map[*workspaceSubscription]struct{})}
}

type workspaceSubscription struct {
	events chan protocol.RuntimeEvent
	// topics is what THIS subscription asked for. The hub broadcasts every signal it
	// receives; a subscription only sees the ones it can fold. resync is not a topic
	// and always passes: a client that fell behind has to be told.
	topics map[protocol.RuntimeTopic]bool
	// sequence is assigned when a frame is handed to the queue, never when one is
	// produced or filtered — so the numbers a subscriber sees are consecutive and a
	// gap can only mean the transport lost a frame.
	sequence uint64
	// stalledTopics / stalledWatchIDs accumulate the scope of signals a full queue
	// could not take. They become one resync; until it is delivered every further
	// signal folds into them, because a client must not receive a later invalidation
	// ahead of the notice that it missed earlier ones.
	stalledTopics   map[protocol.RuntimeTopic]bool
	stalledWatchIDs []string
}

// register adds a caller-owned channel to the broadcast fan-out and returns an
// idempotent unregister. It does NOT close the channel — the owner does, after
// it has stopped every other writer (the file watcher), so a late broadcast
// can't send on a closed channel.
func (h *workspaceHub) register(events chan protocol.RuntimeEvent, topics map[protocol.RuntimeTopic]bool) (*workspaceSubscription, func(), bool) {
	subscription := &workspaceSubscription{events: events, topics: topics}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, nil, false
	}
	h.subscriptions[subscription] = struct{}{}
	h.mu.Unlock()
	return subscription, func() {
		h.mu.Lock()
		delete(h.subscriptions, subscription)
		h.mu.Unlock()
	}, true
}

// closeAdmissions linearizes Server.Close with workspace subscription
// registration. Existing request-owned streams keep running until their own
// contexts end, but once this returns no racing check-then-register path can
// create another subscription.
func (h *workspaceHub) closeAdmissions() {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
}

// publish fans event to every subscriber, folding it into a pending resync for any
// whose buffer is full (see the type doc).
func (h *workspaceHub) publish(event protocol.RuntimeEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscription := range h.subscriptions {
		h.sendLocked(subscription, event)
	}
}

// drained gives a stalled subscription its chance to catch up: the consumer just
// took a frame, so the queue has room the publisher did not have. Without this a
// pending resync would wait for the next unrelated signal, which on a quiet stream
// is never — the loss would be silent, which is the one outcome this hub forbids.
func (h *workspaceHub) drained(subscription *workspaceSubscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subscriptions[subscription]; registered {
		subscription.flushStalledLocked()
	}
}

// publishTo sends a subscription-local event through the same serialization
// point as broadcasts. This keeps each subscriber's sequence strictly ordered
// even when its git watcher races a global workspace event.
func (h *workspaceHub) publishTo(subscription *workspaceSubscription, event protocol.RuntimeEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subscriptions[subscription]; registered {
		h.sendLocked(subscription, event)
	}
}

func (*workspaceHub) sendLocked(subscription *workspaceSubscription, event protocol.RuntimeEvent) {
	if event.Type != protocol.RuntimeResync {
		topic := protocol.RuntimeTopic(event.Type)
		if !slices.Contains(protocol.RuntimeTopics(), topic) {
			// An invalid producer signal has no trustworthy narrowing scope. Preserve
			// client correctness by invalidating everything this subscription holds;
			// the encoder must never silently discard an internal shape violation.
			event = subscription.resyncEvent()
		} else if !subscription.topics[topic] {
			return
		}
	}
	// A pending resync goes first, and while it is pending this signal joins it: the
	// notice that frames were missed must not arrive after the frames that followed
	// them.
	if !subscription.flushStalledLocked() || !subscription.offerLocked(event) {
		subscription.stallLocked(event)
	}
}

// offerLocked hands event to the queue, numbering it only if it fits. Reports whether
// the subscriber has it.
func (subscription *workspaceSubscription) offerLocked(event protocol.RuntimeEvent) bool {
	event = cloneRuntimeEvent(event)
	clearEmptyRuntimeScopes(&event)
	event.Sequence = subscription.sequence + 1
	if err := protocol.ValidateWireTree(event); err != nil {
		event = subscription.resyncEvent()
		event.Sequence = subscription.sequence + 1
		if recoveryErr := protocol.ValidateWireTree(event); recoveryErr != nil {
			panic("server: invalid runtime resync invariant: " + recoveryErr.Error())
		}
	}
	select {
	case subscription.events <- event:
		subscription.sequence++
		return true
	default:
		return false
	}
}

func (subscription *workspaceSubscription) resyncEvent() protocol.RuntimeEvent {
	topics := make([]protocol.RuntimeTopic, 0, len(subscription.topics))
	for _, topic := range protocol.RuntimeTopics() {
		if subscription.topics[topic] {
			topics = append(topics, topic)
		}
	}
	return protocol.RuntimeEvent{Type: protocol.RuntimeResync, Topics: topics}
}

// clearEmptyRuntimeScopes canonicalizes an output value before validation.
// encoding/json omits every empty `omitempty` slice, so nil is the one in-memory
// spelling of absence. Required scopes (files paths and resync topics) remain
// absent and fail their variant rule; optional empty scopes simply disappear.
func clearEmptyRuntimeScopes(event *protocol.RuntimeEvent) {
	if len(event.Paths) == 0 {
		event.Paths = nil
	}
	if len(event.Names) == 0 {
		event.Names = nil
	}
	if len(event.ServerIDs) == 0 {
		event.ServerIDs = nil
	}
	if len(event.ScheduleIDs) == 0 {
		event.ScheduleIDs = nil
	}
	if len(event.SessionIDs) == 0 {
		event.SessionIDs = nil
	}
	if len(event.RunIDs) == 0 {
		event.RunIDs = nil
	}
	if len(event.Topics) == 0 {
		event.Topics = nil
	}
	if len(event.WatchIDs) == 0 {
		event.WatchIDs = nil
	}
}

// stallLocked folds an undeliverable signal's scope into the pending resync. A
// resync being stalled contributes its own scope, so a coalesced resync never
// narrows what an earlier one had already widened.
func (subscription *workspaceSubscription) stallLocked(event protocol.RuntimeEvent) {
	if subscription.stalledTopics == nil {
		subscription.stalledTopics = make(map[protocol.RuntimeTopic]bool, len(protocol.RuntimeTopics()))
	}
	if event.Type == protocol.RuntimeResync {
		for _, topic := range event.Topics {
			subscription.stalledTopics[topic] = true
		}
		subscription.stallWatchIDsLocked(event.WatchIDs)
		return
	}
	subscription.stalledTopics[protocol.RuntimeTopic(event.Type)] = true
	if event.WatchID != "" {
		subscription.stallWatchIDsLocked([]string{event.WatchID})
	}
}

func (subscription *workspaceSubscription) stallWatchIDsLocked(ids []string) {
	for _, id := range ids {
		if id != "" && !slices.Contains(subscription.stalledWatchIDs, id) {
			subscription.stalledWatchIDs = append(subscription.stalledWatchIDs, id)
		}
	}
}

// flushStalledLocked delivers the pending resync if there is one. Reports whether
// the subscription is caught up — false means the queue is still full and the
// caller's own signal has to fold in too.
func (subscription *workspaceSubscription) flushStalledLocked() bool {
	if len(subscription.stalledTopics) == 0 {
		return true
	}
	// Declaration order, not map order: the topics a client is told to re-read are
	// part of the wire, and a set that reshuffles per delivery is a fixture nobody
	// can pin down.
	topics := make([]protocol.RuntimeTopic, 0, len(subscription.stalledTopics))
	for _, topic := range protocol.RuntimeTopics() {
		if subscription.stalledTopics[topic] {
			topics = append(topics, topic)
		}
	}
	if !subscription.offerLocked(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: topics, WatchIDs: subscription.stalledWatchIDs,
	}) {
		return false
	}
	subscription.stalledTopics = nil
	subscription.stalledWatchIDs = nil
	return true
}

// cloneRuntimeEvent gives each subscription sole ownership of every mutable field.
// The hub sends asynchronously-consumed values; sharing the producer's slices would
// let a caller reuse one while a transport is still encoding the event, and sharing
// between subscriptions would let one in-process consumer corrupt another's frame.
//
// Every field is now a slice of ids — a change signal carries no payload — so this is
// a clone of lists and nothing deeper.
func cloneRuntimeEvent(event protocol.RuntimeEvent) protocol.RuntimeEvent {
	event.Paths = slices.Clone(event.Paths)
	event.Names = slices.Clone(event.Names)
	event.ServerIDs = slices.Clone(event.ServerIDs)
	event.ScheduleIDs = slices.Clone(event.ScheduleIDs)
	event.SessionIDs = slices.Clone(event.SessionIDs)
	event.RunIDs = slices.Clone(event.RunIDs)
	event.Topics = slices.Clone(event.Topics)
	event.WatchIDs = slices.Clone(event.WatchIDs)
	return event
}

// SubscribeRuntime opens the change-signal stream (§7.1). The stream's lifetime is
// bounded by the request ctx and by the consumer's range: it ends on client
// disconnect, server shutdown (the transport force-closes the connection), or an
// early range stop.
//
// The caller says which topics it can fold. There is no wildcard: a client that has
// not named a topic cannot be sent it, because a signal it does not understand is a
// refetch it will not perform. Frames for topics it did not ask for are filtered here
// rather than at the producer — one hub, many subscriptions, each with its own set.
//
// When the request carries watches, the subscription also asks the workspace use
// case to monitor those working directories' Git state and emits a debounced
// resync on any change (commit / stage / checkout / merge) — the client then
// re-reads the diff. (Working-tree file
// edits aren't watched directly — see gitWatcher; the agent's own edits arrive as
// files.changed from its tools.)
func (s *Server) SubscribeRuntime(ctx context.Context, request protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	topics, err := s.subscribedTopics(request.Topics)
	if err != nil {
		return nil, nil, err
	}
	workingDirectories, watchIDs, err := validateWorkspaceWatches(request.Watches, topics)
	if err != nil {
		return nil, nil, err
	}

	// SubscribeRuntime owns the channel: the hub broadcasts to it and (when
	// watches are present) the application-owned watcher emits to it. Closing it
	// only after that watcher has stopped keeps emit from racing the close.
	events := make(chan protocol.RuntimeEvent, 64)
	subscription, unregister, registered := s.workspaceHub.register(events, topics)
	if !registered {
		close(events)
		return nil, nil, errSubscriptionAdmissionsClosed
	}

	var fileWatcher io.Closer
	if len(workingDirectories) > 0 {
		fileWatcher, err = s.workspaceWatch.Watch(workingDirectories, func() {
			s.workspaceHub.publishTo(subscription, protocol.RuntimeEvent{
				Type:     protocol.RuntimeResync,
				Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
				WatchIDs: watchIDs,
			})
		})
		if err != nil {
			unregister()
			close(events)
			return nil, nil, mapWorkspaceWatchError(err)
		}
	}

	releaseSubscription := sync.OnceFunc(func() {
		if fileWatcher != nil {
			_ = fileWatcher.Close() // joins callbacks — no emit after this
		}
		unregister() // hub stops broadcasting to events
		close(events)
	})
	stopContextRelease := context.AfterFunc(ctx, releaseSubscription)
	stopSubscription := sync.OnceFunc(func() {
		stopContextRelease()
		releaseSubscription()
	})
	return &protocol.RuntimeSubscribeResponse{}, subscriptionEventSequence(
		events,
		func() { s.workspaceHub.drained(subscription) },
		stopSubscription,
	), nil
}

// subscriptionEventSequence presents a subscription channel as the iter.Seq
// used by the wire streaming contract. The coalescing fan-out hub keeps its
// channels internally; only this outer boundary is a sequence. queueDrained lets
// the hub flush a pending resync, and stopSubscription releases an early-ending
// consumer.
func subscriptionEventSequence(
	events <-chan protocol.RuntimeEvent,
	queueDrained func(),
	stopSubscription func(),
) iter.Seq[protocol.RuntimeEvent] {
	return func(yield func(protocol.RuntimeEvent) bool) {
		defer stopSubscription()
		for event := range events {
			queueDrained()
			if !yield(event) {
				return
			}
		}
	}
}

// subscribedTopics materializes the already shape-validated request as a lookup
// set. The generated RuntimeSubscribeRequest validator owns non-empty/unique
// semantics; this method retains the composition-dependent checks: the enforced
// fan-out cap and whether this runtime actually advertises each topic.
func (s *Server) subscribedTopics(requested []protocol.RuntimeTopic) (map[protocol.RuntimeTopic]bool, error) {
	if len(requested) > protocol.MaxSubscriptionTopics {
		return nil, fmt.Errorf("%w: at most %d topics per subscription", protocol.ErrInvalidParams, protocol.MaxSubscriptionTopics)
	}
	advertised := s.capabilities().RuntimeTopics
	topics := make(map[protocol.RuntimeTopic]bool, len(requested))
	for _, topic := range requested {
		if !slices.Contains(advertised, topic) {
			return nil, protocol.NewCapabilityGap(protocol.CapabilityRequirement{
				Type: protocol.RequirementRuntimeTopic, Name: string(topic),
			})
		}
		topics[topic] = true
	}
	return topics, nil
}

// validateWorkspaceWatches validates the wire-only portion of workspace watches
// and returns the working directories and stable watch identities they name. Root resolution,
// repository layout and filesystem notification are application/adapter concerns;
// Delivery retains only the protocol's required watch identifier.
//
// Watches are legal only with files.changed: the other topics are global, so a watch
// would narrow nothing — and a caller that registered one expecting it to is holding
// a belief the runtime would never honor.
func validateWorkspaceWatches(watches []protocol.WatchSpec, topics map[protocol.RuntimeTopic]bool) (workingDirectories, watchIDs []string, err error) {
	if len(watches) == 0 {
		return nil, nil, nil
	}
	if !topics[protocol.TopicFilesChanged] {
		return nil, nil, fmt.Errorf("%w: watches require the %s topic", protocol.ErrInvalidParams, protocol.TopicFilesChanged)
	}
	if len(watches) > protocol.MaxSubscriptionWatches {
		return nil, nil, fmt.Errorf("%w: at most %d watches per subscription", protocol.ErrInvalidParams, protocol.MaxSubscriptionWatches)
	}
	workingDirectories = make([]string, 0, len(watches))
	watchIDs = make([]string, 0, len(watches))
	for _, spec := range watches {
		if spec.WatchID == "" {
			return nil, nil, fmt.Errorf("%w: watchId is required", protocol.ErrInvalidParams)
		}
		if slices.Contains(watchIDs, spec.WatchID) {
			return nil, nil, fmt.Errorf("%w: watchId %q is registered twice", protocol.ErrInvalidParams, spec.WatchID)
		}
		workingDirectories = append(workingDirectories, spec.Workspace.Path)
		watchIDs = append(watchIDs, spec.WatchID)
	}
	return workingDirectories, watchIDs, nil
}

// mapWorkspaceWatchError reports an unavailable watcher as the precise missing
// capability and delegates all other workspace failures to the shared mapper.
func mapWorkspaceWatchError(err error) error {
	if errors.Is(err, workspaceapp.ErrFileWatchUnavailable) {
		return protocol.NewCapabilityGap(protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature, Name: protocol.FeatureFileWatch,
		})
	}
	return wireWorkspaceError(fmt.Errorf("start git watcher: %w", err))
}
