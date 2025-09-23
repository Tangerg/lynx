// Package maps /* packages
// Thread-safe Map implementations:
//
// 1. SyncMap (Recommended default):
//   - Wraps any Map implementation with RWMutex
//   - Preserves wrapped map's characteristics (order, etc.)
//   - Good balance of performance and flexibility
//   - Use: NewSyncMap() or NewSyncMap(existingMap)
//
// 2. StdSyncMap (High-concurrency reads):
//   - Based on Go's sync.Map
//   - Optimized for read-heavy workloads
//   - No order guarantees
//   - Use: NewStdSyncMap()
//
// Example:
//
//	general := NewSyncMap[string, int]()           // General purpose
//	ordered := NewSyncMap(NewLinkedMap[...])       // Ordered + thread-safe
//	readHeavy := NewStdSyncMap[string, int]()      // Read-optimized
package maps

import (
	"iter"
	"reflect"
	"sync"
)

// SyncMap provides a thread-safe wrapper around any Map implementation.
// It uses a read-write mutex to allow concurrent reads while ensuring
// exclusive access for write operations.
//
// The implementation is designed to minimize lock contention:
//   - Read operations (Get, ContainsKey, Size, etc.) use read locks
//   - Write operations (Put, Remove, etc.) use write locks
//   - Iteration creates a snapshot to avoid holding locks during iteration
type SyncMap[K comparable, V any] struct {
	inner Map[K, V]    // The underlying Map implementation
	mutex sync.RWMutex // Read-write mutex for thread safety
}

// NewSyncMap creates a new thread-safe map wrapper.
// If a Map is provided, it wraps that map; otherwise, it creates a new HashMap.
// If the provided map is already a SyncMap, it returns the same instance
// to avoid double-wrapping.
//
// Examples:
//
//	SyncMap := NewSyncMap[string, int]()                     // wraps a new HashMap
//	SyncMap := NewSyncMap(NewLinkedMap[string, int]())       // wraps a LinkedMap
//	SyncMap := NewSyncMap(existingSyncMap)                   // returns existingSyncMap
//
// The wrapped map should not be accessed directly after wrapping to maintain
// thread safety guarantees.
func NewSyncMap[K comparable, V any](maps ...Map[K, V]) Map[K, V] {
	var inner Map[K, V] = make(HashMap[K, V])

	for _, m := range maps {
		if m != nil {
			// Avoid double-wrapping if the provided map is already thread-safe
			if sm, ok := m.(*SyncMap[K, V]); ok {
				return sm
			}
			if stdSm, ok := m.(*StdSyncMap[K, V]); ok {
				return stdSm
			}
			inner = m.Clone()
			break
		}
	}

	return &SyncMap[K, V]{
		inner: inner,
	}
}

// Basic Operations - Write operations use exclusive locks

// Put safely associates a value with a key with exclusive access.
// Uses a write lock to ensure thread safety.
func (s *SyncMap[K, V]) Put(key K, value V) (V, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Put(key, value)
}

// Remove safely removes a key-value pair with exclusive access.
// Uses a write lock to ensure thread safety.
func (s *SyncMap[K, V]) Remove(key K) (V, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Remove(key)
}

// Basic Operations - Read operations use shared locks

// Get safely retrieves a value with shared access.
// Uses a read lock to allow concurrent reads while preventing
// concurrent writes.
func (s *SyncMap[K, V]) Get(key K) (V, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Get(key)
}

// ContainsKey safely checks key existence with shared access.
// Uses a read lock to allow concurrent reads.
func (s *SyncMap[K, V]) ContainsKey(key K) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.ContainsKey(key)
}

// ContainsValue safely checks value existence with shared access.
// Uses a read lock for the entire operation.
func (s *SyncMap[K, V]) ContainsValue(value V) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.ContainsValue(value)
}

// Query Operations - Read operations use shared locks

// Size safely returns the element count with shared access.
// Uses a read lock to ensure a consistent view.
func (s *SyncMap[K, V]) Size() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Size()
}

// IsEmpty safely checks if the map is empty with shared access.
// Uses a read lock to ensure a consistent view.
func (s *SyncMap[K, V]) IsEmpty() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.IsEmpty()
}

// Bulk Operations

// Clear safely removes all elements with exclusive access.
// Uses a write lock as this is a mutating operation.
func (s *SyncMap[K, V]) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.inner.Clear()
}

// PutAll safely adds multiple key-value pairs with exclusive access.
// The entire operation is atomic - either all applicable pairs
// are added or none are (in case of panic).
func (s *SyncMap[K, V]) PutAll(other Map[K, V]) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.inner.PutAll(other)
}

// Keys safely returns all keys as a snapshot.
// Uses a read lock to ensure a consistent view.
// The returned slice is independent of the map.
func (s *SyncMap[K, V]) Keys() []K {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Keys()
}

// Values safely returns all values as a snapshot.
// Uses a read lock to ensure a consistent view.
// The returned slice is independent of the map.
func (s *SyncMap[K, V]) Values() []V {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Values()
}

// Entries safely returns all key-value pairs as a snapshot.
// Uses a read lock to ensure a consistent view.
// The returned slice is independent of the map.
func (s *SyncMap[K, V]) Entries() []*Entry[K, V] {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Entries()
}

// ForEach safely iterates over all key-value pairs.
// The entire iteration holds a read lock, so the action function
// should be fast to avoid blocking writers for too long.
// For long-running operations, consider using Iter() instead.
func (s *SyncMap[K, V]) ForEach(action func(K, V)) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	s.inner.ForEach(action)
}

// Default Value Related Methods

// GetOrDefault safely gets a value or returns a default with shared access.
// Uses a read lock to ensure consistent reads.
func (s *SyncMap[K, V]) GetOrDefault(key K, defaultValue V) V {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.GetOrDefault(key, defaultValue)
}

// PutIfAbsent safely puts a value only if key is absent with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) PutIfAbsent(key K, value V) (V, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.PutIfAbsent(key, value)
}

// Conditional Operation Methods

// RemoveIf safely removes a key-value pair conditionally with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) RemoveIf(key K, value V) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.RemoveIf(key, value)
}

// Replace safely replaces an existing value with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) Replace(key K, value V) (V, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Replace(key, value)
}

// ReplaceIf safely replaces a value conditionally with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) ReplaceIf(key K, oldValue, newValue V) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.ReplaceIf(key, oldValue, newValue)
}

// Functional Operation Methods

// Compute safely computes a new mapping with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) Compute(key K, remappingFunction func(K, V, bool) (V, bool)) (V, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Compute(key, remappingFunction)
}

// ComputeIfAbsent safely computes a value if absent with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) ComputeIfAbsent(key K, mappingFunction func(K) V) V {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.ComputeIfAbsent(key, mappingFunction)
}

// ComputeIfPresent safely computes a new value if present with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) ComputeIfPresent(key K, remappingFunction func(K, V) V) (V, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.ComputeIfPresent(key, remappingFunction)
}

// Merge safely merges values with exclusive access.
// Uses a write lock as this may be a mutating operation.
func (s *SyncMap[K, V]) Merge(key K, value V, remappingFunction func(V, V) V) V {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Merge(key, value, remappingFunction)
}

// ReplaceAll safely replaces all values with exclusive access.
// Uses a write lock as this is a mutating operation.
func (s *SyncMap[K, V]) ReplaceAll(function func(K, V) V) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.inner.ReplaceAll(function)
}

// Iterator Methods - Use snapshots to avoid holding locks during iteration

// Iter returns a thread-safe iterator by creating a snapshot.
// This approach avoids holding locks during iteration, which could
// cause deadlocks or performance issues with long-running iterations.
//
// The snapshot is taken at the moment Iter() is called, so changes
// made to the map during iteration won't be reflected in the iteration.
// This provides a consistent view but means the iteration might not
// reflect the current state of the map.
//
// Example:
//
//	for k, v := range SyncMap.Iter() {
//	    // This iteration is safe and won't block other operations
//	    fmt.Printf("%v: %v\n", k, v)
//	}
func (s *SyncMap[K, V]) Iter() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		// Create a snapshot with minimal lock time
		s.mutex.RLock()
		entries := s.inner.Entries()
		s.mutex.RUnlock()

		// Iterate over the snapshot without holding any locks
		for _, entry := range entries {
			if !yield(entry.Key(), entry.Value()) {
				return
			}
		}
	}
}

// IterKeys returns a thread-safe key iterator using snapshots.
// See Iter() for details about snapshot-based iteration.
func (s *SyncMap[K, V]) IterKeys() iter.Seq[K] {
	return func(yield func(K) bool) {
		// Create a snapshot with minimal lock time
		s.mutex.RLock()
		keys := s.inner.Keys()
		s.mutex.RUnlock()

		// Iterate over the snapshot without holding any locks
		for _, key := range keys {
			if !yield(key) {
				return
			}
		}
	}
}

// IterValues returns a thread-safe value iterator using snapshots.
// See Iter() for details about snapshot-based iteration.
func (s *SyncMap[K, V]) IterValues() iter.Seq[V] {
	return func(yield func(V) bool) {
		// Create a snapshot with minimal lock time
		s.mutex.RLock()
		values := s.inner.Values()
		s.mutex.RUnlock()

		// Iterate over the snapshot without holding any locks
		for _, value := range values {
			if !yield(value) {
				return
			}
		}
	}
}

// Clone safely creates an independent copy of the map.
// The clone is also a SyncMap wrapping a copy of the inner map.
// Uses a read lock to ensure a consistent snapshot for cloning.
func (s *SyncMap[K, V]) Clone() Map[K, V] {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	cloned := s.inner.Clone()
	return &SyncMap[K, V]{
		inner: cloned,
	}
}

// StdSyncMap is a thread-safe Map interface implementation based on Go's standard library sync.Map.
// It provides concurrent access safety for all operations, making it suitable
// for use in multi-goroutine environments. The underlying sync.Map is optimized
// for scenarios where entries are only ever written once but read many times,
// or when multiple goroutines read, write, and overwrite entries for disjoint sets of keys.
type StdSyncMap[K comparable, V any] struct {
	m sync.Map
}

// NewStdSyncMap creates a new thread-safe StdSyncMap instance.
func NewStdSyncMap[K comparable, V any]() *StdSyncMap[K, V] {
	return &StdSyncMap[K, V]{}
}

// Put associates the specified value with the specified key in this map.
// If the map previously contained a mapping for the key, the old value is replaced.
// This operation is thread-safe and can be called concurrently from multiple goroutines.
func (s *StdSyncMap[K, V]) Put(key K, value V) (V, bool) {
	if oldValue, exists := s.m.Swap(key, value); exists {
		return oldValue.(V), true
	}
	var zero V
	return zero, false
}

// Get returns the value to which the specified key is mapped.
// This operation is thread-safe and optimized for concurrent reads.
func (s *StdSyncMap[K, V]) Get(key K) (V, bool) {
	if value, exists := s.m.Load(key); exists {
		return value.(V), true
	}
	var zero V
	return zero, false
}

// Remove removes the mapping for a key from this map if it is present.
// This operation is thread-safe and can be called concurrently.
func (s *StdSyncMap[K, V]) Remove(key K) (V, bool) {
	if value, exists := s.m.LoadAndDelete(key); exists {
		return value.(V), true
	}
	var zero V
	return zero, false
}

// ContainsKey returns true if this map contains a mapping for the specified key.
// This operation is thread-safe.
func (s *StdSyncMap[K, V]) ContainsKey(key K) bool {
	_, exists := s.m.Load(key)
	return exists
}

// ContainsValue returns true if this map maps one or more keys to the specified value.
// This operation requires scanning all entries and uses reflection for deep equality comparison.
// Note: This operation is expensive and may impact performance in concurrent scenarios.
func (s *StdSyncMap[K, V]) ContainsValue(value V) bool {
	found := false
	s.m.Range(func(key, val any) bool {
		if reflect.DeepEqual(val.(V), value) {
			found = true
			return false // Stop iteration
		}
		return true // Continue iteration
	})
	return found
}

// Size returns the number of key-value mappings in this map.
// Note: This operation requires scanning all entries, which may be expensive.
func (s *StdSyncMap[K, V]) Size() int {
	count := 0
	s.m.Range(func(key, value any) bool {
		count++
		return true
	})
	return count
}

// IsEmpty returns true if this map contains no key-value mappings.
// This operation checks for the existence of at least one entry.
func (s *StdSyncMap[K, V]) IsEmpty() bool {
	isEmpty := true
	s.m.Range(func(key, value any) bool {
		isEmpty = false
		return false // Stop iteration after finding first entry
	})
	return isEmpty
}

// Clear removes all of the mappings from this map.
// This operation creates a new sync.Map instance to ensure thread safety.
func (s *StdSyncMap[K, V]) Clear() {
	s.m.Clear()
}

// PutAll copies all of the mappings from the specified map to this map.
// This operation is thread-safe but may not be atomic across all entries.
func (s *StdSyncMap[K, V]) PutAll(other Map[K, V]) {
	other.ForEach(func(k K, v V) {
		s.m.Store(k, v)
	})
}

// Keys returns a slice containing all the keys in this map.
// The returned slice is a snapshot of the current keys at the time of the call.
func (s *StdSyncMap[K, V]) Keys() []K {
	var keys []K
	s.m.Range(func(key, value any) bool {
		keys = append(keys, key.(K))
		return true
	})
	return keys
}

// Values returns a slice containing all the values in this map.
// The returned slice is a snapshot of the current values at the time of the call.
func (s *StdSyncMap[K, V]) Values() []V {
	var values []V
	s.m.Range(func(key, value any) bool {
		values = append(values, value.(V))
		return true
	})
	return values
}

// Entries returns a slice containing all the key-value pairs in this map.
// Each entry is represented as a pointer to an Entry struct.
// The returned slice is a snapshot of the current entries at the time of the call.
func (s *StdSyncMap[K, V]) Entries() []*Entry[K, V] {
	var entries []*Entry[K, V]
	s.m.Range(func(key, value any) bool {
		entries = append(entries, &Entry[K, V]{
			key:   key.(K),
			value: value.(V),
		})
		return true
	})
	return entries
}

// ForEach performs the given action for each key-value pair in this map.
// The action function is called once for each mapping in the map.
// Note: The iteration order is not guaranteed and may vary between calls.
func (s *StdSyncMap[K, V]) ForEach(action func(K, V)) {
	s.m.Range(func(key, value any) bool {
		action(key.(K), value.(V))
		return true
	})
}

// GetOrDefault returns the value to which the specified key is mapped,
// or defaultValue if this map contains no mapping for the key.
func (s *StdSyncMap[K, V]) GetOrDefault(key K, defaultValue V) V {
	if value, exists := s.m.Load(key); exists {
		return value.(V)
	}
	return defaultValue
}

// PutIfAbsent associates the specified value with the specified key only if
// the key is not already associated with a value.
// This operation is atomic and thread-safe.
func (s *StdSyncMap[K, V]) PutIfAbsent(key K, value V) (V, bool) {
	actual, loaded := s.m.LoadOrStore(key, value)
	if loaded {
		return actual.(V), false // Key already existed
	}
	return value, true // Key was absent, value was stored
}

// RemoveIf removes the entry for the specified key only if it is currently
// mapped to the specified value using deep equality comparison.
// This operation uses compare-and-swap semantics for thread safety.
func (s *StdSyncMap[K, V]) RemoveIf(key K, value V) bool {
	if currentValue, exists := s.m.Load(key); exists {
		if reflect.DeepEqual(currentValue.(V), value) {
			// Use CompareAndDelete for atomic operation
			return s.m.CompareAndDelete(key, currentValue)
		}
	}
	return false
}

// Replace replaces the entry for the specified key only if it is currently mapped to some value.
// This operation uses compare-and-swap semantics for thread safety.
func (s *StdSyncMap[K, V]) Replace(key K, value V) (V, bool) {
	if oldValue, exists := s.m.Load(key); exists {
		if s.m.CompareAndSwap(key, oldValue, value) {
			return oldValue.(V), true
		}
		// If CompareAndSwap failed, the value was changed by another goroutine
		// Try to get the current value
		if currentValue, stillExists := s.m.Load(key); stillExists {
			return currentValue.(V), false
		}
	}
	var zero V
	return zero, false
}

// ReplaceIf replaces the entry for the specified key only if currently mapped to the specified value.
// This operation uses compare-and-swap semantics for thread safety.
func (s *StdSyncMap[K, V]) ReplaceIf(key K, oldValue, newValue V) bool {
	if currentValue, exists := s.m.Load(key); exists {
		if reflect.DeepEqual(currentValue.(V), oldValue) {
			return s.m.CompareAndSwap(key, currentValue, newValue)
		}
	}
	return false
}

// Compute attempts to compute a mapping for the specified key and its current mapped value.
// This operation is not atomic across the entire computation but provides consistency guarantees.
// The remappingFunc receives the key, current value, and existence flag.
func (s *StdSyncMap[K, V]) Compute(key K, remappingFunc func(K, V, bool) (V, bool)) (V, bool) {
	for {
		oldValue, exists := s.m.Load(key)
		var currentValue V
		if exists {
			currentValue = oldValue.(V)
		}

		newValue, shouldPut := remappingFunc(key, currentValue, exists)

		if shouldPut {
			if exists {
				// Try to replace the existing value
				if s.m.CompareAndSwap(key, oldValue, newValue) {
					return newValue, true
				}
				// Value was changed by another goroutine, retry
				continue
			} else {
				// Try to store the new value
				if _, loaded := s.m.LoadOrStore(key, newValue); !loaded {
					return newValue, true
				}
				// Another goroutine stored a value, retry
				continue
			}
		} else {
			if exists {
				// Try to delete the existing value
				if s.m.CompareAndDelete(key, oldValue) {
					var zero V
					return zero, false
				}
				// Value was changed by another goroutine, retry
				continue
			}
			// Key doesn't exist and we don't want to put anything
			var zero V
			return zero, false
		}
	}
}

// ComputeIfAbsent computes a value for the specified key if the key is not already
// associated with a value, and associates it with the computed value.
// This operation is atomic and thread-safe.
func (s *StdSyncMap[K, V]) ComputeIfAbsent(key K, mappingFunction func(K) V) V {
	if value, exists := s.m.Load(key); exists {
		return value.(V)
	}

	newValue := mappingFunction(key)
	actual, _ := s.m.LoadOrStore(key, newValue)
	return actual.(V)
}

// ComputeIfPresent computes a new mapping for the specified key if the key is
// currently mapped to a value in this map.
// This operation uses compare-and-swap for thread safety.
func (s *StdSyncMap[K, V]) ComputeIfPresent(key K, remappingFunc func(K, V) V) (V, bool) {
	for {
		if oldValue, exists := s.m.Load(key); exists {
			newValue := remappingFunc(key, oldValue.(V))
			if s.m.CompareAndSwap(key, oldValue, newValue) {
				return newValue, true
			}
			// Value was changed by another goroutine, retry
			continue
		}
		// Key doesn't exist
		var zero V
		return zero, false
	}
}

// Merge associates the specified value with the specified key if the key is not
// already associated with a value, or merges the existing value with the new value
// using the provided remapping function.
// This operation handles concurrency using compare-and-swap semantics.
func (s *StdSyncMap[K, V]) Merge(key K, value V, remappingFunc func(V, V) V) V {
	for {
		if oldValue, exists := s.m.Load(key); exists {
			newValue := remappingFunc(oldValue.(V), value)
			if s.m.CompareAndSwap(key, oldValue, newValue) {
				return newValue
			}
			// Value was changed by another goroutine, retry
			continue
		} else {
			// Key doesn't exist, try to store the new value
			if _, loaded := s.m.LoadOrStore(key, value); !loaded {
				return value
			}
			// Another goroutine stored a value, retry with merge
			continue
		}
	}
}

// ReplaceAll replaces each entry's value with the result of invoking the given
// function on that entry's key and value.
// Note: This operation is not atomic across all entries but each individual
// replacement uses compare-and-swap for consistency.
func (s *StdSyncMap[K, V]) ReplaceAll(function func(K, V) V) {
	// First, collect all current entries
	var entries []struct {
		key      K
		oldValue any
	}

	s.m.Range(func(key, value any) bool {
		entries = append(entries, struct {
			key      K
			oldValue any
		}{key.(K), value})
		return true
	})

	// Then, try to replace each entry
	for _, entry := range entries {
		for {
			// Check if the entry still exists and has the same value
			if currentValue, exists := s.m.Load(entry.key); exists && currentValue == entry.oldValue {
				newValue := function(entry.key, entry.oldValue.(V))
				if s.m.CompareAndSwap(entry.key, entry.oldValue, newValue) {
					break // Successfully replaced
				}
				// Value was changed, get the new value and retry
				if newCurrentValue, stillExists := s.m.Load(entry.key); stillExists {
					entry.oldValue = newCurrentValue
					continue
				}
				// Entry was deleted, skip it
				break
			}
			// Entry was changed or deleted, skip it
			break
		}
	}
}

// Iter returns an iterator that yields key-value pairs.
// Note: StdSyncMap does not guarantee any specific iteration order.
// The iteration represents a snapshot at the time Iter() is called,
// but the underlying map may be modified during iteration by other goroutines.
func (s *StdSyncMap[K, V]) Iter() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		s.m.Range(func(key, value any) bool {
			return yield(key.(K), value.(V))
		})
	}
}

// IterKeys returns an iterator that yields keys only.
// Note: StdSyncMap does not guarantee any specific iteration order.
// The iteration represents a snapshot at the time IterKeys() is called.
func (s *StdSyncMap[K, V]) IterKeys() iter.Seq[K] {
	return func(yield func(K) bool) {
		s.m.Range(func(key, _ any) bool {
			return yield(key.(K))
		})
	}
}

// IterValues returns an iterator that yields values only.
// Note: StdSyncMap does not guarantee any specific iteration order.
// The iteration represents a snapshot at the time IterValues() is called.
func (s *StdSyncMap[K, V]) IterValues() iter.Seq[V] {
	return func(yield func(V) bool) {
		s.m.Range(func(_, value any) bool {
			return yield(value.(V))
		})
	}
}

// Clone creates an independent copy of the StdSyncMap.
// The cloned map contains the same key-value pairs but is a separate instance.
// This operation is not atomic - the clone represents a snapshot of the map
// at the time Clone() is called, but concurrent modifications may result in
// an inconsistent snapshot.
func (s *StdSyncMap[K, V]) Clone() Map[K, V] {
	cloned := NewStdSyncMap[K, V]()
	cloned.PutAll(s)
	return cloned
}
