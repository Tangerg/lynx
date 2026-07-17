package core

import (
	"fmt"
	"sync"
)

// memoryStore is the goroutine-safe map backend shared by the reference
// [ProcessStore] and [SessionStore] implementations. Both concrete backends
// need id-keyed save, load, delete, and list primitives even though their
// public persistence contracts differ.
//
// label appears in error messages ("memory process store: ..." vs
// "memory session store: ...") so callers see which surface
// rejected an empty id or returned a not-found error.
type memoryStore[V any] struct {
	label    string
	notFound error

	mu    sync.RWMutex
	items map[string]V
}

func newMemoryStore[V any](label string, notFound error) *memoryStore[V] {
	return &memoryStore[V]{
		label:    label,
		notFound: notFound,
		items:    map[string]V{},
	}
}

// save rejects empty ids so callers don't silently overwrite the zero
// key; the id-rejection invariant matches both SPIs.
func (store *memoryStore[V]) save(id string, value V) error {
	if id == "" {
		return fmt.Errorf("%s: id must not be empty", store.label)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.items[id] = value
	return nil
}

// load wraps notFound (a sentinel supplied at construction) so
// callers can `errors.Is` against it without caring about the backend.
func (store *memoryStore[V]) load(id string) (V, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	value, ok := store.items[id]
	if !ok {
		var zero V
		return zero, fmt.Errorf("%s: %w (id=%q)", store.label, store.notFound, id)
	}
	return value, nil
}

func (store *memoryStore[V]) delete(id string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.items, id)
}

func (store *memoryStore[V]) list() []string {
	store.mu.RLock()
	defer store.mu.RUnlock()
	ids := make([]string, 0, len(store.items))
	for id := range store.items {
		ids = append(ids, id)
	}
	return ids
}
