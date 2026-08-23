// Package identitylane provides cancellable FIFO serialization for exact
// business identities. The registry mutex is never held while waiting for or
// executing I/O, so unrelated identities remain concurrent.
package identitylane

import (
	"context"
	"slices"
	"sync"
)

type Coordinator struct {
	mu    sync.Mutex
	lanes map[string]*lane
}

type lane struct {
	token chan struct{}
	refs  int
}

func New() *Coordinator {
	return &Coordinator{lanes: make(map[string]*lane)}
}

func (coordinator *Coordinator) Acquire(ctx context.Context, keys ...string) (func(), error) {
	keys = slices.Clone(keys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	coordinator.mu.Lock()
	lanes := make([]*lane, len(keys))
	for index, key := range keys {
		entry := coordinator.lanes[key]
		if entry == nil {
			entry = &lane{token: make(chan struct{}, 1)}
			entry.token <- struct{}{}
			coordinator.lanes[key] = entry
		}
		entry.refs++
		lanes[index] = entry
	}
	coordinator.mu.Unlock()

	acquired := 0
	for index, entry := range lanes {
		select {
		case <-ctx.Done():
			coordinator.release(keys, lanes, acquired)
			return nil, ctx.Err()
		case <-entry.token:
			acquired = index + 1
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() { coordinator.release(keys, lanes, len(lanes)) })
	}, nil
}

func (coordinator *Coordinator) release(keys []string, lanes []*lane, acquired int) {
	for index := acquired - 1; index >= 0; index-- {
		lanes[index].token <- struct{}{}
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	for index, entry := range lanes {
		entry.refs--
		if entry.refs == 0 {
			delete(coordinator.lanes, keys[index])
		}
	}
}
