package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// errServerClosed reports that a request-detached delivery operation could not
// start because the Server is shutting down (its task group is closed).
var errServerClosed = errors.New("server: closed")

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
	mu     sync.Mutex
	subs   map[*workspaceSubscription]struct{}
	closed bool
}

func newWorkspaceHub() *workspaceHub {
	return &workspaceHub{subs: make(map[*workspaceSubscription]struct{})}
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
func (h *workspaceHub) register(ch chan protocol.RuntimeEvent, topics map[protocol.RuntimeTopic]bool) (*workspaceSubscription, func(), bool) {
	sub := &workspaceSubscription{events: ch, topics: topics}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, nil, false
	}
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub, func() {
		h.mu.Lock()
		delete(h.subs, sub)
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

// observe wires the run pump's live file-change nudges (delivered through the
// composition-root bridge) into the hub: each nudge becomes a files.changed signal
// fanned to subscribers. The wire shape stays here in delivery; the bridge itself
// carries only neutral (cwd, paths).
func (h *workspaceHub) observe(src Source[runs.FileChange]) {
	src.Observe(func(change runs.FileChange) {
		h.publish(protocol.RuntimeEvent{
			Type:  protocol.RuntimeFilesChanged,
			Cwd:   change.Cwd,
			Paths: change.Paths,
		})
	})
}

// publish fans ev to every subscriber, folding it into a pending resync for any
// whose buffer is full (see the type doc).
func (h *workspaceHub) publish(ev protocol.RuntimeEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		h.sendLocked(sub, ev)
	}
}

// drained gives a stalled subscription its chance to catch up: the consumer just
// took a frame, so the queue has room the publisher did not have. Without this a
// pending resync would wait for the next unrelated signal, which on a quiet stream
// is never — the loss would be silent, which is the one outcome this hub forbids.
func (h *workspaceHub) drained(sub *workspaceSubscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subs[sub]; registered {
		sub.flushStalledLocked()
	}
}

// publishTo sends a subscription-local event through the same serialization
// point as broadcasts. This keeps each subscriber's sequence strictly ordered
// even when its git watcher races a global workspace event.
func (h *workspaceHub) publishTo(sub *workspaceSubscription, ev protocol.RuntimeEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subs[sub]; registered {
		h.sendLocked(sub, ev)
	}
}

func (*workspaceHub) sendLocked(sub *workspaceSubscription, ev protocol.RuntimeEvent) {
	if ev.Type != protocol.RuntimeResync {
		topic := protocol.RuntimeTopic(ev.Type)
		if !slices.Contains(protocol.RuntimeTopics, topic) {
			// An invalid producer signal has no trustworthy narrowing scope. Preserve
			// client correctness by invalidating everything this subscription holds;
			// the encoder must never silently discard an internal shape violation.
			ev = sub.resyncEvent()
		} else if !sub.topics[topic] {
			return
		}
	}
	// A pending resync goes first, and while it is pending this signal joins it: the
	// notice that frames were missed must not arrive after the frames that followed
	// them.
	if !sub.flushStalledLocked() || !sub.offerLocked(ev) {
		sub.stallLocked(ev)
	}
}

// offerLocked hands ev to the queue, numbering it only if it fits. Reports whether
// the subscriber has it.
func (sub *workspaceSubscription) offerLocked(ev protocol.RuntimeEvent) bool {
	ev = cloneRuntimeEvent(ev)
	clearEmptyRuntimeScopes(&ev)
	ev.Sequence = sub.sequence + 1
	if err := ev.ValidateWire(); err != nil {
		ev = sub.resyncEvent()
		ev.Sequence = sub.sequence + 1
		if recoveryErr := ev.ValidateWire(); recoveryErr != nil {
			panic("server: invalid runtime resync invariant: " + recoveryErr.Error())
		}
	}
	select {
	case sub.events <- ev:
		sub.sequence++
		return true
	default:
		return false
	}
}

func (sub *workspaceSubscription) resyncEvent() protocol.RuntimeEvent {
	topics := make([]protocol.RuntimeTopic, 0, len(sub.topics))
	for _, topic := range protocol.RuntimeTopics {
		if sub.topics[topic] {
			topics = append(topics, topic)
		}
	}
	return protocol.RuntimeEvent{Type: protocol.RuntimeResync, Topics: topics}
}

// clearEmptyRuntimeScopes canonicalizes an output value before validation.
// encoding/json omits every empty `omitempty` slice, so nil is the one in-memory
// spelling of absence. Required scopes (files paths and resync topics) remain
// absent and fail their variant rule; optional empty scopes simply disappear.
func clearEmptyRuntimeScopes(ev *protocol.RuntimeEvent) {
	if len(ev.Paths) == 0 {
		ev.Paths = nil
	}
	if len(ev.Names) == 0 {
		ev.Names = nil
	}
	if len(ev.ServerIDs) == 0 {
		ev.ServerIDs = nil
	}
	if len(ev.ScheduleIDs) == 0 {
		ev.ScheduleIDs = nil
	}
	if len(ev.SessionIDs) == 0 {
		ev.SessionIDs = nil
	}
	if len(ev.RunIDs) == 0 {
		ev.RunIDs = nil
	}
	if len(ev.Topics) == 0 {
		ev.Topics = nil
	}
	if len(ev.WatchIDs) == 0 {
		ev.WatchIDs = nil
	}
}

// stallLocked folds an undeliverable signal's scope into the pending resync. A
// resync being stalled contributes its own scope, so a coalesced resync never
// narrows what an earlier one had already widened.
func (sub *workspaceSubscription) stallLocked(ev protocol.RuntimeEvent) {
	if sub.stalledTopics == nil {
		sub.stalledTopics = make(map[protocol.RuntimeTopic]bool, len(protocol.RuntimeTopics))
	}
	if ev.Type == protocol.RuntimeResync {
		for _, topic := range ev.Topics {
			sub.stalledTopics[topic] = true
		}
		sub.stallWatchIDsLocked(ev.WatchIDs)
		return
	}
	sub.stalledTopics[protocol.RuntimeTopic(ev.Type)] = true
	if ev.WatchID != "" {
		sub.stallWatchIDsLocked([]string{ev.WatchID})
	}
}

func (sub *workspaceSubscription) stallWatchIDsLocked(ids []string) {
	for _, id := range ids {
		if id != "" && !slices.Contains(sub.stalledWatchIDs, id) {
			sub.stalledWatchIDs = append(sub.stalledWatchIDs, id)
		}
	}
}

// flushStalledLocked delivers the pending resync if there is one. Reports whether
// the subscription is caught up — false means the queue is still full and the
// caller's own signal has to fold in too.
func (sub *workspaceSubscription) flushStalledLocked() bool {
	if len(sub.stalledTopics) == 0 {
		return true
	}
	// Declaration order, not map order: the topics a client is told to re-read are
	// part of the wire, and a set that reshuffles per delivery is a fixture nobody
	// can pin down.
	topics := make([]protocol.RuntimeTopic, 0, len(sub.stalledTopics))
	for _, topic := range protocol.RuntimeTopics {
		if sub.stalledTopics[topic] {
			topics = append(topics, topic)
		}
	}
	if !sub.offerLocked(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: topics, WatchIDs: sub.stalledWatchIDs,
	}) {
		return false
	}
	sub.stalledTopics = nil
	sub.stalledWatchIDs = nil
	return true
}

// cloneRuntimeEvent gives each subscription sole ownership of every mutable field.
// The hub sends asynchronously-consumed values; sharing the producer's slices would
// let a caller reuse one while a transport is still encoding the event, and sharing
// between subscriptions would let one in-process consumer corrupt another's frame.
//
// Every field is now a slice of ids — a change signal carries no payload — so this is
// a clone of lists and nothing deeper.
func cloneRuntimeEvent(ev protocol.RuntimeEvent) protocol.RuntimeEvent {
	ev.Paths = slices.Clone(ev.Paths)
	ev.Names = slices.Clone(ev.Names)
	ev.ServerIDs = slices.Clone(ev.ServerIDs)
	ev.ScheduleIDs = slices.Clone(ev.ScheduleIDs)
	ev.SessionIDs = slices.Clone(ev.SessionIDs)
	ev.RunIDs = slices.Clone(ev.RunIDs)
	ev.Topics = slices.Clone(ev.Topics)
	ev.WatchIDs = slices.Clone(ev.WatchIDs)
	return ev
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
// When the request carries watches, the subscription also asks the workspace use case
// to monitor those cwds' Git state and emits a debounced resync on any change (commit
// / stage / checkout / merge) — the client then re-reads the diff. (Working-tree file
// edits aren't watched directly — see gitWatcher; the agent's own edits arrive as
// files.changed from its tools.)
func (s *Server) SubscribeRuntime(ctx context.Context, in protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	topics, err := s.subscribedTopics(in.Topics)
	if err != nil {
		return nil, nil, err
	}
	cwds, watchIDs, err := watchCwds(in.Watches, topics)
	if err != nil {
		return nil, nil, err
	}

	// SubscribeRuntime owns the channel: the hub broadcasts to it and (when
	// watches are present) the application-owned watcher emits to it. Closing it
	// only after that watcher has stopped keeps emit from racing the close.
	out := make(chan protocol.RuntimeEvent, 64)
	subscription, unregister, registered := s.wsHub.register(out, topics)
	if !registered {
		close(out)
		return nil, nil, errServerClosed
	}

	var watcher io.Closer
	if len(cwds) > 0 {
		watcher, err = s.workspaceWatch.WatchGitState(cwds, func() {
			s.wsHub.publishTo(subscription, protocol.RuntimeEvent{
				Type:     protocol.RuntimeResync,
				Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
				WatchIDs: watchIDs,
			})
		})
		if err != nil {
			unregister()
			close(out)
			return nil, nil, mapWorkspaceSubscribeError(err)
		}
	}

	release := sync.OnceFunc(func() {
		if watcher != nil {
			_ = watcher.Close() // joins callbacks — no emit after this
		}
		unregister() // hub stops broadcasting to out
		close(out)
	})
	stopContextRelease := context.AfterFunc(ctx, release)
	stop := sync.OnceFunc(func() {
		stopContextRelease()
		release()
	})
	return &protocol.RuntimeSubscribeResponse{}, eventSeq(out, func() { s.wsHub.drained(subscription) }, stop), nil
}

// eventSeq presents a subscription channel as the iter.Seq the wire streaming
// contract uses. The coalescing fan-out hub keeps its channels internally; only
// this outer boundary is a sequence. drained tells the hub a frame left the queue,
// which is when a subscription that fell behind can be handed its resync. stop
// releases the subscription when the consumer ends the range before its request
// context does.
func eventSeq(ch <-chan protocol.RuntimeEvent, drained, stop func()) iter.Seq[protocol.RuntimeEvent] {
	return func(yield func(protocol.RuntimeEvent) bool) {
		defer stop()
		for ev := range ch {
			drained()
			if !yield(ev) {
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
	advertised := s.Capabilities().RuntimeTopics
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

// watchCwds validates the wire-only portion of watch specs. Root resolution,
// repository layout and filesystem notification are application/adapter concerns;
// Delivery retains only the protocol's required watch identifier.
//
// Watches are legal only with files.changed: the other topics are global, so a watch
// would narrow nothing — and a caller that registered one expecting it to is holding
// a belief the runtime would never honor.
func watchCwds(specs []protocol.WatchSpec, topics map[protocol.RuntimeTopic]bool) (cwds, watchIDs []string, err error) {
	if len(specs) == 0 {
		return nil, nil, nil
	}
	if !topics[protocol.TopicFilesChanged] {
		return nil, nil, fmt.Errorf("%w: watches require the %s topic", protocol.ErrInvalidParams, protocol.TopicFilesChanged)
	}
	if len(specs) > protocol.MaxSubscriptionWatches {
		return nil, nil, fmt.Errorf("%w: at most %d watches per subscription", protocol.ErrInvalidParams, protocol.MaxSubscriptionWatches)
	}
	cwds = make([]string, 0, len(specs))
	watchIDs = make([]string, 0, len(specs))
	for _, spec := range specs {
		if spec.WatchID == "" {
			return nil, nil, fmt.Errorf("%w: watchId is required", protocol.ErrInvalidParams)
		}
		if slices.Contains(watchIDs, spec.WatchID) {
			return nil, nil, fmt.Errorf("%w: watchId %q is registered twice", protocol.ErrInvalidParams, spec.WatchID)
		}
		cwds = append(cwds, spec.Cwd)
		watchIDs = append(watchIDs, spec.WatchID)
	}
	return cwds, watchIDs, nil
}

// mapWorkspaceSubscribeError refuses a watch this build cannot serve by NAMING the
// missing capability. It used to name the method instead, which told the client
// where it was standing rather than what it lacked — and after the method was
// renamed, it named a method that no longer exists.
func mapWorkspaceSubscribeError(err error) error {
	if errors.Is(err, workspaceapp.ErrFileWatchUnavailable) {
		return protocol.NewCapabilityGap(protocol.CapabilityRequirement{
			Type: protocol.RequirementFeature, Name: protocol.FeatureFileWatch,
		})
	}
	return wireWorkspaceError(fmt.Errorf("start git watcher: %w", err))
}

// PublishRuntimeEvent fans one workspace event out to subscribers. The
// runtime / engine call this when a non-run state change happens (mcp
// serverChanged, skills.changed, files.changed). Safe to call with no
// subscribers (no-op).
func (s *Server) PublishRuntimeEvent(ev protocol.RuntimeEvent) {
	if s.wsHub != nil {
		s.wsHub.publish(ev)
	}
}
