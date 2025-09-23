package maps

import (
	"iter"
)

// Map defines the basic interface for key-value pair mappings.
// This interface provides comprehensive operations for managing key-value associations,
// including basic CRUD operations, bulk operations, functional programming methods,
// and advanced conditional operations similar to Java's Map interface.
type Map[K comparable, V any] interface {
	// Put associates the specified value with the specified key in this map.
	// If the map previously contained a mapping for the key, the old value is replaced.
	// Returns the previous value associated with key, and true if there was a previous mapping.
	Put(key K, value V) (V, bool)

	// Get returns the value to which the specified key is mapped.
	// Returns the value associated with the key and true if the key exists,
	// otherwise returns zero value and false.
	Get(key K) (V, bool)

	// Remove removes the mapping for a key from this map if it is present.
	// Returns the value that was associated with the key and true if the key existed,
	// otherwise returns zero value and false.
	Remove(key K) (V, bool)

	// ContainsKey returns true if this map contains a mapping for the specified key.
	ContainsKey(key K) bool

	// ContainsValue returns true if this map maps one or more keys to the specified value.
	// This operation typically requires time linear in the map size for most implementations.
	ContainsValue(value V) bool

	// Size returns the number of key-value mappings in this map.
	Size() int

	// IsEmpty returns true if this map contains no key-value mappings.
	IsEmpty() bool

	// Clear removes all of the mappings from this map.
	// The map will be empty after this call returns.
	Clear()

	// PutAll copies all of the mappings from the specified map to this map.
	// These mappings will replace any mappings that this map had for any of the keys
	// currently in the specified map.
	PutAll(other Map[K, V])

	// Keys returns a slice containing all the keys in this map.
	// The slice is a snapshot; changes to the map are not reflected in the slice.
	Keys() []K

	// Values returns a slice containing all the values in this map.
	// The slice is a snapshot; changes to the map are not reflected in the slice.
	Values() []V

	// Entries returns a slice containing all the key-value pairs in this map.
	// Each entry is represented as an Entry struct containing the key and value.
	Entries() []*Entry[K, V]

	// ForEach performs the given action for each key-value pair in this map.
	// The action function is called once for each mapping in the map.
	ForEach(action func(K, V))

	// GetOrDefault returns the value to which the specified key is mapped,
	// or defaultValue if this map contains no mapping for the key.
	GetOrDefault(key K, defaultValue V) V

	// PutIfAbsent associates the specified value with the specified key only if
	// the key is not already associated with a value.
	// Returns the current value associated with the key and false if the key was already present,
	// or the new value and true if the key was absent.
	PutIfAbsent(key K, value V) (V, bool)

	// RemoveIf removes the entry for the specified key only if it is currently
	// mapped to the specified value. Returns true if the entry was removed.
	RemoveIf(key K, value V) bool

	// Replace replaces the entry for the specified key only if it is currently mapped to some value.
	// Returns the previous value associated with the key and true if replacement occurred,
	// otherwise returns zero value and false.
	Replace(key K, value V) (V, bool)

	// ReplaceIf replaces the entry for the specified key only if currently mapped to the specified value.
	// Returns true if the value was replaced.
	ReplaceIf(key K, oldValue, newValue V) bool

	// Compute attempts to compute a mapping for the specified key and its current mapped value
	// (or null if there is no current mapping). The remappingFunction receives the key,
	// current value, and whether the key exists. If the function returns a value and true,
	// the mapping is updated; if it returns false, the mapping is removed if it exists.
	Compute(key K, remappingFunction func(K, V, bool) (V, bool)) (V, bool)

	// ComputeIfAbsent computes a value for the specified key if the key is not already
	// associated with a value, and associates it with the computed value.
	// Returns the current (existing or computed) value associated with the key.
	ComputeIfAbsent(key K, mappingFunction func(K) V) V

	// ComputeIfPresent computes a new mapping for the specified key if the key is
	// currently mapped to a value in this map. Returns the new value and true if
	// the mapping was updated, otherwise returns zero value and false.
	ComputeIfPresent(key K, remappingFunction func(K, V) V) (V, bool)

	// Merge associates the specified value with the specified key if the key is not
	// already associated with a value. If the key is already associated with a value,
	// replaces the associated value with the results of the given remapping function.
	// Returns the new value associated with the key.
	Merge(key K, value V, remappingFunction func(V, V) V) V

	// ReplaceAll replaces each entry's value with the result of invoking the given
	// function on that entry until all entries have been processed.
	ReplaceAll(function func(K, V) V)

	// Iter returns an iterator that yields key-value pairs.
	// The iteration order depends on the specific implementation:
	//   - HashMap: unordered, may vary between iterations
	//   - LinkedMap: insertion order
	//   - TreeMap: sorted order (if implemented)
	Iter() iter.Seq2[K, V]

	// IterKeys returns an iterator that yields keys only.
	// The iteration order follows the same rules as Iter().
	IterKeys() iter.Seq[K]

	// IterValues returns an iterator that yields values only.
	// The iteration order follows the same rules as Iter().
	IterValues() iter.Seq[V]

	// Clone creates an independent copy of this map.
	// The cloned map contains the same key-value pairs but is a separate instance.
	// Changes to the original map will not affect the clone and vice versa.
	// Note: This typically performs a shallow copy.
	Clone() Map[K, V]
}

// Entry represents a key-value pair entry in the map.
// This struct encapsulates a single mapping from the map, providing
// immutable access to both the key and value components.
type Entry[K comparable, V any] struct {
	key   K
	value V
}

// Key returns the key corresponding to this entry.
func (e *Entry[K, V]) Key() K {
	return e.key
}

// Value returns the value corresponding to this entry.
func (e *Entry[K, V]) Value() V {
	return e.value
}
