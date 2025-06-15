package maps

import (
	"iter"
	"reflect"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// HashMap is a Map interface implementation based on Go's built-in map.
// It provides constant-time performance for basic operations (get and put),
// assuming the hash function disperses elements properly among the buckets.
// This implementation is not synchronized and is not thread-safe.
type HashMap[K comparable, V any] map[K]V

// NewHashMap create a new HashMap instance
func NewHashMap[K comparable, V any](size ...int) HashMap[K, V] {
	c, _ := pkgSlices.First(size)
	if c <= 0 {
		c = 0
	}
	return make(HashMap[K, V], c)
}

// Put associates the specified value with the specified key in this map.
// If the map previously contained a mapping for the key, the old value is replaced.
func (h HashMap[K, V]) Put(key K, value V) (V, bool) {
	oldValue, exists := h[key]
	h[key] = value
	return oldValue, exists
}

// Get returns the value to which the specified key is mapped.
func (h HashMap[K, V]) Get(key K) (V, bool) {
	value, exists := h[key]
	return value, exists
}

// Remove removes the mapping for a key from this map if it is present.
func (h HashMap[K, V]) Remove(key K) (V, bool) {
	value, exists := h[key]
	if exists {
		delete(h, key)
	}
	return value, exists
}

// ContainsKey returns true if this map contains a mapping for the specified key.
func (h HashMap[K, V]) ContainsKey(key K) bool {
	_, exists := h[key]
	return exists
}

// ContainsValue returns true if this map maps one or more keys to the specified value.
// This method uses reflection for deep equality comparison, which may impact performance.
func (h HashMap[K, V]) ContainsValue(value V) bool {
	for _, v := range h {
		if reflect.DeepEqual(v, value) {
			return true
		}
	}
	return false
}

// Size returns the number of key-value mappings in this map.
func (h HashMap[K, V]) Size() int {
	return len(h)
}

// IsEmpty returns true if this map contains no key-value mappings.
func (h HashMap[K, V]) IsEmpty() bool {
	return len(h) == 0
}

// Clear removes all of the mappings from this map using Go's built-in clear function.
func (h HashMap[K, V]) Clear() {
	clear(h)
}

// PutAll copies all of the mappings from the specified map to this map.
func (h HashMap[K, V]) PutAll(other Map[K, V]) {
	other.ForEach(func(k K, v V) {
		h[k] = v
	})
}

// Keys returns a slice containing all the keys in this map.
// The returned slice is a snapshot of the current keys.
func (h HashMap[K, V]) Keys() []K {
	keys := make([]K, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

// Values returns a slice containing all the values in this map.
// The returned slice is a snapshot of the current values.
func (h HashMap[K, V]) Values() []V {
	values := make([]V, 0, len(h))
	for _, v := range h {
		values = append(values, v)
	}
	return values
}

// Entries returns a slice containing all the key-value pairs in this map.
// Each entry is represented as a pointer to an Entry struct.
func (h HashMap[K, V]) Entries() []*Entry[K, V] {
	entries := make([]*Entry[K, V], 0, len(h))
	for k, v := range h {
		entries = append(entries, &Entry[K, V]{
			key:   k,
			value: v,
		})
	}
	return entries
}

// ForEach performs the given action for each key-value pair in this map.
func (h HashMap[K, V]) ForEach(action func(K, V)) {
	for k, v := range h {
		action(k, v)
	}
}

// GetOrDefault returns the value to which the specified key is mapped,
// or defaultValue if this map contains no mapping for the key.
func (h HashMap[K, V]) GetOrDefault(key K, defaultValue V) V {
	if value, exists := h[key]; exists {
		return value
	}
	return defaultValue
}

// PutIfAbsent associates the specified value with the specified key only if
// the key is not already associated with a value.
func (h HashMap[K, V]) PutIfAbsent(key K, value V) (V, bool) {
	if existingValue, exists := h[key]; exists {
		return existingValue, false
	}
	h[key] = value
	return value, true
}

// RemoveIf removes the entry for the specified key only if it is currently
// mapped to the specified value using deep equality comparison.
func (h HashMap[K, V]) RemoveIf(key K, value V) bool {
	if existingValue, exists := h[key]; exists && reflect.DeepEqual(existingValue, value) {
		delete(h, key)
		return true
	}
	return false
}

// Replace replaces the entry for the specified key only if it is currently mapped to some value.
func (h HashMap[K, V]) Replace(key K, value V) (V, bool) {
	if oldValue, exists := h[key]; exists {
		h[key] = value
		return oldValue, true
	}
	var zero V
	return zero, false
}

// ReplaceIf replaces the entry for the specified key only if currently mapped to the specified value.
func (h HashMap[K, V]) ReplaceIf(key K, oldValue, newValue V) bool {
	if existingValue, exists := h[key]; exists && reflect.DeepEqual(existingValue, oldValue) {
		h[key] = newValue
		return true
	}
	return false
}

// Compute attempts to compute a mapping for the specified key and its current mapped value.
// The remappingFunc receives the key, current value, and existence flag.
func (h HashMap[K, V]) Compute(key K, remappingFunc func(K, V, bool) (V, bool)) (V, bool) {
	oldValue, exists := h[key]
	newValue, shouldPut := remappingFunc(key, oldValue, exists)

	if shouldPut {
		h[key] = newValue
		return newValue, true
	} else if exists {
		delete(h, key)
	}

	var zero V
	return zero, false
}

// ComputeIfAbsent computes a value for the specified key if the key is not already
// associated with a value, and associates it with the computed value.
func (h HashMap[K, V]) ComputeIfAbsent(key K, mappingFunction func(K) V) V {
	if value, exists := h[key]; exists {
		return value
	}

	newValue := mappingFunction(key)
	h[key] = newValue
	return newValue
}

// ComputeIfPresent computes a new mapping for the specified key if the key is
// currently mapped to a value in this map.
func (h HashMap[K, V]) ComputeIfPresent(key K, remappingFunc func(K, V) V) (V, bool) {
	if oldValue, exists := h[key]; exists {
		newValue := remappingFunc(key, oldValue)
		h[key] = newValue
		return newValue, true
	}

	var zero V
	return zero, false
}

// Merge associates the specified value with the specified key if the key is not
// already associated with a value, or merges the existing value with the new value
// using the provided remapping function.
func (h HashMap[K, V]) Merge(key K, value V, remappingFunc func(V, V) V) V {
	if oldValue, exists := h[key]; exists {
		newValue := remappingFunc(oldValue, value)
		h[key] = newValue
		return newValue
	}

	h[key] = value
	return value
}

// ReplaceAll replaces each entry's value with the result of invoking the given
// function on that entry's key and value.
func (h HashMap[K, V]) ReplaceAll(function func(K, V) V) {
	for k, v := range h {
		h[k] = function(k, v)
	}
}

// Iter returns an iterator that yields key-value pairs.
// Note: HashMap does not guarantee any specific iteration order.
// The order may vary between different iterations and Go versions.
//
// Example:
//
//	for k, v := range HashMap.Iter() {
//		fmt.Printf("%v: %v\n", k, v)
//	}
func (h HashMap[K, V]) Iter() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range h {
			if !yield(k, v) {
				return
			}
		}
	}
}

// IterKeys returns an iterator that yields keys only.
// Note: HashMap does not guarantee any specific iteration order.
// The order may vary between different iterations and Go versions.
//
// Example:
//
//	for k := range HashMap.IterKeys() {
//		fmt.Printf("Key: %v\n", k)
//	}
func (h HashMap[K, V]) IterKeys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range h {
			if !yield(k) {
				return
			}
		}
	}
}

// IterValues returns an iterator that yields values only.
// Note: HashMap does not guarantee any specific iteration order.
// The order may vary between different iterations and Go versions.
//
// Example:
//
//	for v := range HashMap.IterValues() {
//		fmt.Printf("Value: %v\n", v)
//	}
func (h HashMap[K, V]) IterValues() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range h {
			if !yield(v) {
				return
			}
		}
	}
}

// Clone creates an independent copy of the HashMap.
// The cloned map contains the same key-value pairs but is a separate instance.
// Changes to the original map will not affect the clone and vice versa.
//
// Note: This performs a shallow copy - if values contain pointers,
// the pointed-to data is shared between original and clone.
//
// Example:
//
//	original := HashMap[string, int]{"a": 1, "b": 2}
//	cloned := original.Clone()
//	cloned.Put("c", 3) // Only affects the clone
func (h HashMap[K, V]) Clone() Map[K, V] {
	cloned := NewHashMap[K, V](h.Size())
	cloned.PutAll(h)
	return cloned
}
