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

type runKey struct{ rootRunID, rootSegmentID string }

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

// PublishRun sends a committed replayable fact or an explicitly ephemeral
// preview. A lagging consumer is detached instead of blocking the Runtime; its
// next subscription replays authoritative facts and resumes fresh previews.
func (hub *Hub) PublishRun(rootRunID, rootSegmentID string, event protocol.RunEvent) {
	if rootRunID == "" || rootSegmentID == "" || event.RunID == "" || event.SegmentID == "" {
		return
	}
	key := runKey{rootRunID: rootRunID, rootSegmentID: rootSegmentID}
	terminal := event.RunID == rootRunID && event.SegmentID == rootSegmentID &&
		event.Event.Type == protocol.StreamSegmentFinished
	hub.mu.Lock()
	for id, subscriber := range hub.runSubs[key] {
		select {
		case subscriber.values <- event:
			if terminal {
				subscriber.close()
				delete(hub.runSubs[key], id)
			}
		default:
			subscriber.close()
			delete(hub.runSubs[key], id)
		}
	}
	if terminal || len(hub.runSubs[key]) == 0 {
		delete(hub.runSubs, key)
	}
	hub.mu.Unlock()
}

func (hub *Hub) SubscribeRun(
	ctx context.Context,
	rootRunID, rootSegmentID string,
	replay []protocol.RunEvent,
) iter.Seq[protocol.RunEvent] {
	key := runKey{rootRunID: rootRunID, rootSegmentID: rootSegmentID}
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
				// PublishRun closes a terminal subscription only after enqueueing
				// segment.finished. Drain the already ordered queue before exit so
				// selecting done cannot lose the final authoritative frame.
				for {
					select {
					case event := <-subscriber.values:
						if !yield(event) {
							return
						}
					default:
						return
					}
				}
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
