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

// identityCoordinator serializes only operations that contend for the same
// physical or domain identity. Its mutex owns the lane registry; cancellable
// tokens own I/O ordering, so unrelated resources never wait behind global I/O.
type identityCoordinator struct {
	mu    sync.Mutex
	lanes map[string]*identityLane
}

type identityLane struct {
	token chan struct{}
	refs  int
}

func newIdentityCoordinator() *identityCoordinator {
	return &identityCoordinator{lanes: make(map[string]*identityLane)}
}

func (coordinator *identityCoordinator) Acquire(
	ctx context.Context,
	keys ...string,
) (func(), error) {
	slices.Sort(keys)
	keys = slices.Compact(keys)
	coordinator.mu.Lock()
	lanes := make([]*identityLane, len(keys))
	for index, key := range keys {
		lane := coordinator.lanes[key]
		if lane == nil {
			lane = &identityLane{token: make(chan struct{}, 1)}
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

func (coordinator *identityCoordinator) release(
	keys []string,
	lanes []*identityLane,
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
