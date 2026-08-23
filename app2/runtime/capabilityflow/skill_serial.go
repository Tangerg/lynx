package capabilityflow

import (
	"context"
	"slices"
	"sync"
)

const userSkillLibraryLane = "library\x00user"

func skillProposalLane(workspace, name string) string {
	return "proposal\x00" + workspace + "\x00" + name
}

// skillCoordinator serializes only operations that contend for the same user
// library or immutable proposal identity. Its mutex owns the lane registry;
// cancellable lane tokens own I/O ordering, so unrelated project Skills never
// wait behind one global lock.
type skillCoordinator struct {
	mu    sync.Mutex
	lanes map[string]*skillLane
}

type skillLane struct {
	token chan struct{}
	refs  int
}

func newSkillCoordinator() *skillCoordinator {
	return &skillCoordinator{lanes: make(map[string]*skillLane)}
}

func (coordinator *skillCoordinator) Acquire(
	ctx context.Context,
	keys ...string,
) (func(), error) {
	slices.Sort(keys)
	keys = slices.Compact(keys)
	coordinator.mu.Lock()
	lanes := make([]*skillLane, len(keys))
	for index, key := range keys {
		lane := coordinator.lanes[key]
		if lane == nil {
			lane = &skillLane{token: make(chan struct{}, 1)}
			lane.token <- struct{}{}
			coordinator.lanes[key] = lane
		}
		lane.refs++
		lanes[index] = lane
	}
	coordinator.mu.Unlock()

	acquired := 0
	for index, lane := range lanes {
		select {
		case <-ctx.Done():
			coordinator.release(keys, lanes, acquired)
			return nil, ctx.Err()
		case <-lane.token:
			acquired = index + 1
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() { coordinator.release(keys, lanes, len(lanes)) })
	}, nil
}

func (coordinator *skillCoordinator) release(
	keys []string,
	lanes []*skillLane,
	acquired int,
) {
	for index := acquired - 1; index >= 0; index-- {
		lanes[index].token <- struct{}{}
	}
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	for index, lane := range lanes {
		lane.refs--
		if lane.refs == 0 {
			delete(coordinator.lanes, keys[index])
		}
	}
}
