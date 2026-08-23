// Package streamhub owns live fan-out for the two concrete Lyra streams. The
// durable stores remain replay truth; this hub only bridges committed facts to
// currently attached consumers.
package streamhub

import (
	"context"
	"iter"
	"sync"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

const subscriberCapacity = 64

type runKey struct{ runID, segmentID string }

type runSubscriber struct {
	values chan protocol.RunEvent
	done   chan struct{}
	once   sync.Once
}

func newRunSubscriber() *runSubscriber {
	return &runSubscriber{values: make(chan protocol.RunEvent, subscriberCapacity), done: make(chan struct{})}
}

func (subscriber *runSubscriber) close() { subscriber.once.Do(func() { close(subscriber.done) }) }

type Hub struct {
	mu      sync.Mutex
	nextID  uint64
	runSubs map[runKey]map[uint64]*runSubscriber
	closed  bool
}

func New() *Hub { return &Hub{runSubs: make(map[runKey]map[uint64]*runSubscriber)} }

// PublishRun sends an already committed event. A lagging consumer is detached
// instead of dropping an authoritative frame; it reconnects through durable
// replay using its last event id.
func (hub *Hub) PublishRun(event protocol.RunEvent) {
	key := runKey{runID: event.RunID, segmentID: event.SegmentID}
	hub.mu.Lock()
	for id, subscriber := range hub.runSubs[key] {
		select {
		case subscriber.values <- event:
		default:
			subscriber.close()
			delete(hub.runSubs[key], id)
		}
	}
	hub.mu.Unlock()
}

func (hub *Hub) SubscribeRun(
	ctx context.Context,
	runID, segmentID string,
	replay []protocol.RunEvent,
) iter.Seq[protocol.RunEvent] {
	key := runKey{runID: runID, segmentID: segmentID}
	subscriber := newRunSubscriber()
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		subscriber.close()
	} else {
		hub.nextID++
		id := hub.nextID
		if hub.runSubs[key] == nil {
			hub.runSubs[key] = make(map[uint64]*runSubscriber)
		}
		hub.runSubs[key][id] = subscriber
		hub.mu.Unlock()
		context.AfterFunc(ctx, func() {
			hub.mu.Lock()
			if entries := hub.runSubs[key]; entries != nil {
				delete(entries, id)
				if len(entries) == 0 {
					delete(hub.runSubs, key)
				}
			}
			subscriber.close()
			hub.mu.Unlock()
		})
	}
	return func(yield func(protocol.RunEvent) bool) {
		for _, event := range replay {
			if !yield(event) {
				return
			}
		}
		for {
			select {
			case event := <-subscriber.values:
				if !yield(event) {
					return
				}
			case <-subscriber.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}
}

func (hub *Hub) Close() {
	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return
	}
	hub.closed = true
	for _, subscribers := range hub.runSubs {
		for _, subscriber := range subscribers {
			subscriber.close()
		}
	}
	hub.runSubs = nil
	hub.mu.Unlock()
}
