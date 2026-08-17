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

func (g *invocationGroup) Attach(parent context.Context) (context.Context, func(), bool) {
	ctx, cancel := context.WithCancel(parent)
	g.mu.Lock()
	if g.stopping || g.lifetime.Err() != nil {
		g.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	g.nextID++
	id := g.nextID
	g.active[id] = cancel
	g.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			g.mu.Lock()
			delete(g.active, id)
			g.finishShutdownLocked()
			g.mu.Unlock()
		})
	}
	return ctx, release, true
}

// BeginShutdown closes admission and broadcasts cancellation before waiting on
// any one operation. Calls already inside their source remain registered until
// they actually return.
func (g *invocationGroup) BeginShutdown() {
	g.mu.Lock()
	g.stopping = true
	cancels := make([]context.CancelFunc, 0, len(g.active))
	for _, cancel := range g.active {
		cancels = append(cancels, cancel)
	}
	g.finishShutdownLocked()
	g.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

func (g *invocationGroup) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("operation: shutdown context is required")
	}
	select {
	case <-g.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *invocationGroup) finishShutdownLocked() {
	if !g.stopping || len(g.active) != 0 || g.finished {
		return
	}
	close(g.done)
	g.finished = true
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
