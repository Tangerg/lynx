package operation

import (
	"context"
	"errors"
	"iter"
	"sync"
)

// invocationGroup owns Endpoint admission and the exact lifetime of every
// accepted unary call or stream. It is delivery-local: application task groups
// continue to own request-detached product work, while this group prevents the
// process owner from closing their dependencies underneath an active binding
// call.
type invocationGroup struct {
	lifetime context.Context

	mu       sync.Mutex
	stopping bool
	finished bool
	nextID   uint64
	active   map[uint64]context.CancelFunc
	done     chan struct{}
}

func newInvocationGroup(lifetime context.Context) *invocationGroup {
	group := &invocationGroup{
		lifetime: lifetime,
		active:   make(map[uint64]context.CancelFunc),
		done:     make(chan struct{}),
	}
	context.AfterFunc(lifetime, group.BeginShutdown)
	return group
}

func (i *invocationGroup) Attach(parent context.Context) (context.Context, func(), bool) {
	ctx, cancel := context.WithCancel(parent)
	i.mu.Lock()
	if i.stopping || i.lifetime.Err() != nil {
		i.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	i.nextID++
	id := i.nextID
	i.active[id] = cancel
	i.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			i.mu.Lock()
			delete(i.active, id)
			i.finishShutdownLocked()
			i.mu.Unlock()
		})
	}
	return ctx, release, true
}

// BeginShutdown closes admission and broadcasts cancellation before waiting on
// any one operation. Calls already inside their source remain registered until
// they actually return.
func (i *invocationGroup) BeginShutdown() {
	i.mu.Lock()
	i.stopping = true
	cancels := make([]context.CancelFunc, 0, len(i.active))
	for _, cancel := range i.active {
		cancels = append(cancels, cancel)
	}
	i.finishShutdownLocked()
	i.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (i *invocationGroup) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("operation: shutdown context is required")
	}
	select {
	case <-i.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *invocationGroup) finishShutdownLocked() {
	if !i.stopping || len(i.active) != 0 || i.finished {
		return
	}
	close(i.done)
	i.finished = true
}

// ownStream keeps one accepted operation registered until its source returns.
// If a caller never starts ranging, cancellation claims and ranges the source
// itself with a rejecting yield. That joins source-side teardown instead of
// merely assuming every context.AfterFunc callback has already completed.
func ownStream(
	ctx context.Context,
	events iter.Seq2[any, error],
	release func(),
) iter.Seq2[any, error] {
	var (
		mu      sync.Mutex
		claimed bool
	)
	finish := sync.OnceFunc(release)
	run := func(yield func(any, error) bool) {
		defer finish()
		events(yield)
	}

	stopAbandon := context.AfterFunc(ctx, func() {
		mu.Lock()
		if claimed {
			mu.Unlock()
			return
		}
		claimed = true
		mu.Unlock()
		run(func(any, error) bool { return false })
	})

	return func(yield func(any, error) bool) {
		mu.Lock()
		if claimed {
			mu.Unlock()
			return
		}
		claimed = true
		mu.Unlock()
		stopAbandon()
		run(yield)
	}
}
