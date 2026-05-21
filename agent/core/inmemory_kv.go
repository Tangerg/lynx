package core

import (
	"fmt"
	"sync"
)

// inMemoryKV is the goroutine-safe map backend shared by the
// reference [ProcessStore] and [SessionStore] implementations — both
// SPIs have the same Save / Load / Delete / List shape over an
// id-keyed value, so the storage layer is one type with two
// instantiations rather than two near-identical copies.
//
// label appears in error messages ("in-memory process store: ..." vs
// "in-memory session store: ...") so callers see which surface
// rejected an empty id or returned a not-found error.
type inMemoryKV[V any] struct {
	label    string
	notFound error

	mu    sync.RWMutex
	items map[string]V
}

func newInMemoryKV[V any](label string, notFound error) *inMemoryKV[V] {
	return &inMemoryKV[V]{
		label:    label,
		notFound: notFound,
		items:    map[string]V{},
	}
}

// save rejects empty ids so callers don't silently overwrite the zero
// key; the id-rejection invariant matches both SPIs.
func (kv *inMemoryKV[V]) save(id string, value V) error {
	if id == "" {
		return fmt.Errorf("%s: id must not be empty", kv.label)
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.items[id] = value
	return nil
}

// load wraps notFound (a sentinel supplied at construction) so
// callers can `errors.Is` against it without caring about the backend.
func (kv *inMemoryKV[V]) load(id string) (V, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	v, ok := kv.items[id]
	if !ok {
		var zero V
		return zero, fmt.Errorf("%s: %w (id=%q)", kv.label, kv.notFound, id)
	}
	return v, nil
}

func (kv *inMemoryKV[V]) delete(id string) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	delete(kv.items, id)
}

func (kv *inMemoryKV[V]) list() []string {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	ids := make([]string, 0, len(kv.items))
	for id := range kv.items {
		ids = append(ids, id)
	}
	return ids
}

