package tool

import (
	"context"
	"maps"
	"sync"
)

// Context implements the Context interface with thread-safe key-value storage.
type Context struct {
	ctx    context.Context
	mu     sync.RWMutex   // Protects concurrent access to fields
	fields map[string]any // Thread-safe key-value storage
}

// NewContext creates a new thread-safe Context instance with the provided Context.Context.
func NewContext(ctx context.Context) *Context {
	return &Context{
		ctx:    ctx,
		fields: make(map[string]any),
	}
}

func (t *Context) Context() context.Context {
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

func (t *Context) Set(key string, val any) *Context {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.fields[key] = val
	return t
}

func (t *Context) SetMap(m map[string]any) *Context {
	if len(m) == 0 {
		return t
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for k, v := range m {
		t.fields[k] = v
	}
	return t
}

func (t *Context) Get(key string) (any, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	val, ok := t.fields[key]
	return val, ok
}

func (t *Context) Fields() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return maps.Clone(t.fields)
}

func (t *Context) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	clear(t.fields)
}

func (t *Context) Clone() *Context {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &Context{
		ctx:    t.ctx,
		fields: maps.Clone(t.fields),
	}
}
