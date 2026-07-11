// Package taskgroup owns cancelable, request-detached work for a process
// component. It provides the lifecycle boundary a component uses to launch,
// cancel, and join its own background tasks without knowing what those tasks
// do — the process-component scope shared across the application, delivery, and
// composition rings.
package taskgroup

import (
	"context"
	"sync"
)

// Group starts request-detached tasks and cancels and joins them at Close.
// The zero value is ready to use. Start and Close are safe to call
// concurrently; once closed, a Group cannot be reused.
type Group struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	closed  bool
	nextID  uint64
	cancels map[uint64]context.CancelFunc
}

// Start launches task with parent values but without parent cancellation. It
// returns false when task is nil or the group is already closed.
func (g *Group) Start(parent context.Context, task func(context.Context)) bool {
	if task == nil {
		return false
	}
	ctx, release, ok := g.Attach(parent)
	if !ok {
		return false
	}

	go func() {
		defer release()
		task(ctx)
	}()
	return true
}

// Attach registers caller-managed work with the group. The returned context
// preserves parent values, ignores parent cancellation, and is canceled by
// Close. The caller must release it when the work ends; release is idempotent.
func (g *Group) Attach(parent context.Context) (ctx context.Context, release func(), ok bool) {
	return g.attach(context.WithoutCancel(parent))
}

// AttachLinked registers caller-managed work whose context is canceled by
// either the parent or Close. Use it for inbound calls owned jointly by the
// caller and the component; background maintenance should use Attach instead.
func (g *Group) AttachLinked(parent context.Context) (ctx context.Context, release func(), ok bool) {
	return g.attach(parent)
}

func (g *Group) attach(parent context.Context) (ctx context.Context, release func(), ok bool) {
	ctx, cancel := context.WithCancel(parent)

	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	if g.cancels == nil {
		g.cancels = map[uint64]context.CancelFunc{}
	}
	g.nextID++
	id := g.nextID
	g.cancels[id] = cancel
	g.wg.Add(1)
	g.mu.Unlock()

	var once sync.Once
	release = func() {
		once.Do(func() { g.finish(id, cancel) })
	}
	return ctx, release, true
}

func (g *Group) finish(id uint64, cancel context.CancelFunc) {
	cancel()
	g.mu.Lock()
	delete(g.cancels, id)
	g.mu.Unlock()
	g.wg.Done()
}

// Close rejects new tasks, cancels active tasks, and waits for them to return.
// It is safe to call repeatedly.
func (g *Group) Close() {
	g.mu.Lock()
	g.closed = true
	cancels := make([]context.CancelFunc, 0, len(g.cancels))
	for _, cancel := range g.cancels {
		cancels = append(cancels, cancel)
	}
	g.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
	g.wg.Wait()
}
