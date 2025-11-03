package maps

import (
	"iter"
	"reflect"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// mapNode represents a node in the doubly-linked list for LinkedMap.
// Each node contains a key-value pair and pointers to maintain insertion order.
type mapNode[K comparable, V any] struct {
	key   K
	value V
	prev  *mapNode[K, V]
	next  *mapNode[K, V]
}

// LinkedMap is a Map implementation that maintains insertion order.
// It combines a hash map for O(1) lookups with a doubly-linked list
// for maintaining and iterating over key-value pairs in insertion order.
//
// This implementation provides:
//   - O(1) lookup, insertion, and deletion
//   - Predictable iteration order (insertion order)
//   - Memory overhead for storing linked list pointers
//   - All standard Map interface operations
type LinkedMap[K comparable, V any] struct {
	nodes map[K]*mapNode[K, V] // Maps keys to their linked list nodes for O(1) access
	head  *mapNode[K, V]       // First key-value pair in insertion order
	tail  *mapNode[K, V]       // Last key-value pair in insertion order
}

// NewLinkedMap create a new LinkedMap instance
func NewLinkedMap[K comparable, V any](size ...int) *LinkedMap[K, V] {
	c, _ := pkgSlices.First(size)
	if c <= 0 {
		c = 0
	}
	return &LinkedMap[K, V]{
		nodes: make(map[K]*mapNode[K, V], c),
	}
}

// Put associates the specified value with the specified key in this map.
// If the map previously contained a mapping for the key, the old value is replaced
// but the insertion order position is preserved.
// Time complexity: O(1)
func (l *LinkedMap[K, V]) Put(key K, value V) (V, bool) {
	// Check if key already exists
	if existingNode, exists := l.nodes[key]; exists {
		oldValue := existingNode.value
		existingNode.value = value // Update value, preserve position
		return oldValue, true
	}

	// Create new node and add to hash map
	newNode := &mapNode[K, V]{key: key, value: value}
	l.nodes[key] = newNode

	// Insert at the end of the linked list
	if l.tail == nil {
		// Empty list - both head and tail point to the new node
		l.head = newNode
		l.tail = newNode
	} else {
		// Non-empty list - append to tail
		l.tail.next = newNode
		newNode.prev = l.tail
		l.tail = newNode
	}

	var zero V
	return zero, false
}

// Get returns the value to which the specified key is mapped.
// Time complexity: O(1)
func (l *LinkedMap[K, V]) Get(key K) (V, bool) {
	if node, exists := l.nodes[key]; exists {
		return node.value, true
	}
	var zero V
	return zero, false
}

// Remove removes the mapping for a key from this map if it is present.
// Time complexity: O(1)
func (l *LinkedMap[K, V]) Remove(key K) (V, bool) {
	// Find the node using the hash map for O(1) lookup
	nodeToRemove, exists := l.nodes[key]
	if !exists {
		var zero V
		return zero, false
	}

	oldValue := nodeToRemove.value

	// Remove from hash map
	delete(l.nodes, key)

	// Remove from linked list structure
	l.removeNode(nodeToRemove)

	return oldValue, true
}

// removeNode removes a node from the doubly-linked list and updates head/tail pointers.
// This is a helper method that handles all the pointer manipulation safely.
func (l *LinkedMap[K, V]) removeNode(node *mapNode[K, V]) {
	// Update previous node's next pointer
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		// Removing the head node
		l.head = node.next
	}

	// Update next node's previous pointer
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		// Removing the tail node
		l.tail = node.prev
	}

	// Clear the removed node's pointers to prevent memory leaks
	node.prev = nil
	node.next = nil
}

// ContainsKey returns true if this map contains a mapping for the specified key.
// Time complexity: O(1)
func (l *LinkedMap[K, V]) ContainsKey(key K) bool {
	_, exists := l.nodes[key]
	return exists
}

// ContainsValue returns true if this map maps one or more keys to the specified value.
// This operation requires scanning all entries and uses reflection for deep equality comparison.
// Time complexity: O(n)
func (l *LinkedMap[K, V]) ContainsValue(value V) bool {
	current := l.head
	for current != nil {
		if reflect.DeepEqual(current.value, value) {
			return true
		}
		current = current.next
	}
	return false
}

// Size returns the number of key-value mappings in this map.
// Time complexity: O(1)
func (l *LinkedMap[K, V]) Size() int {
	return len(l.nodes)
}

// IsEmpty returns true if this map contains no key-value mappings.
// Time complexity: O(1)
func (l *LinkedMap[K, V]) IsEmpty() bool {
	return l.Size() == 0
}

// Clear removes all of the mappings from this map.
// Carefully clears all node pointers to prevent memory leaks.
func (l *LinkedMap[K, V]) Clear() {
	// Walk through the linked list and clear all node pointers
	current := l.head
	for current != nil {
		next := current.next
		current.prev = nil
		current.next = nil
		current = next
	}

	// Reset the data structures
	clear(l.nodes)
	l.head = nil
	l.tail = nil
}

// PutAll copies all of the mappings from the specified map to this map.
// New mappings are added at the end in the order they are encountered.
// Existing keys have their values updated but preserve their insertion order position.
func (l *LinkedMap[K, V]) PutAll(other Map[K, V]) {
	other.ForEach(func(k K, v V) {
		l.Put(k, v)
	})
}

// Keys returns a slice containing all the keys in this map in insertion order.
// The returned slice is a snapshot of the current keys.
func (l *LinkedMap[K, V]) Keys() []K {
	keys := make([]K, 0, l.Size())
	current := l.head
	for current != nil {
		keys = append(keys, current.key)
		current = current.next
	}
	return keys
}

// Values returns a slice containing all the values in this map in insertion order.
// The returned slice is a snapshot of the current values.
func (l *LinkedMap[K, V]) Values() []V {
	values := make([]V, 0, l.Size())
	current := l.head
	for current != nil {
		values = append(values, current.value)
		current = current.next
	}
	return values
}

// Entries returns a slice containing all the key-value pairs in this map in insertion order.
// Each entry is represented as a pointer to an Entry struct.
func (l *LinkedMap[K, V]) Entries() []*Entry[K, V] {
	entries := make([]*Entry[K, V], 0, l.Size())
	current := l.head
	for current != nil {
		entries = append(entries, &Entry[K, V]{
			key:   current.key,
			value: current.value,
		})
		current = current.next
	}
	return entries
}

// ForEach performs the given action for each key-value pair in this map in insertion order.
// The action function is called once for each mapping in the map.
func (l *LinkedMap[K, V]) ForEach(action func(K, V)) {
	current := l.head
	for current != nil {
		action(current.key, current.value)
		current = current.next
	}
}

// GetOrDefault returns the value to which the specified key is mapped,
// or defaultValue if this map contains no mapping for the key.
func (l *LinkedMap[K, V]) GetOrDefault(key K, defaultValue V) V {
	if value, exists := l.Get(key); exists {
		return value
	}
	return defaultValue
}

// PutIfAbsent associates the specified value with the specified key only if
// the key is not already associated with a value.
// If the key is absent, it's added at the end of the insertion order.
func (l *LinkedMap[K, V]) PutIfAbsent(key K, value V) (V, bool) {
	if existingValue, exists := l.Get(key); exists {
		return existingValue, false
	}
	l.Put(key, value)
	return value, true
}

// RemoveIf removes the entry for the specified key only if it is currently
// mapped to the specified value using deep equality comparison.
func (l *LinkedMap[K, V]) RemoveIf(key K, value V) bool {
	if node, exists := l.nodes[key]; exists {
		if reflect.DeepEqual(node.value, value) {
			l.Remove(key)
			return true
		}
	}
	return false
}

// Replace replaces the entry for the specified key only if it is currently mapped to some value.
// The insertion order position is preserved.
func (l *LinkedMap[K, V]) Replace(key K, value V) (V, bool) {
	if node, exists := l.nodes[key]; exists {
		oldValue := node.value
		node.value = value
		return oldValue, true
	}
	var zero V
	return zero, false
}

// ReplaceIf replaces the entry for the specified key only if currently mapped to the specified value.
func (l *LinkedMap[K, V]) ReplaceIf(key K, oldValue, newValue V) bool {
	if node, exists := l.nodes[key]; exists {
		if reflect.DeepEqual(node.value, oldValue) {
			node.value = newValue
			return true
		}
	}
	return false
}

// Compute attempts to compute a mapping for the specified key and its current mapped value.
// The remappingFunc receives the key, current value, and existence flag.
func (l *LinkedMap[K, V]) Compute(key K, remappingFunc func(K, V, bool) (V, bool)) (V, bool) {
	currentValue, exists := l.Get(key)
	newValue, shouldPut := remappingFunc(key, currentValue, exists)

	if shouldPut {
		l.Put(key, newValue)
		return newValue, true
	} else if exists {
		l.Remove(key)
	}

	var zero V
	return zero, false
}

// ComputeIfAbsent computes a value for the specified key if the key is not already
// associated with a value, and associates it with the computed value.
// The new mapping is added at the end of the insertion order.
func (l *LinkedMap[K, V]) ComputeIfAbsent(key K, mappingFunction func(K) V) V {
	if value, exists := l.Get(key); exists {
		return value
	}

	newValue := mappingFunction(key)
	l.Put(key, newValue)
	return newValue
}

// ComputeIfPresent computes a new mapping for the specified key if the key is
// currently mapped to a value in this map.
// The insertion order position is preserved.
func (l *LinkedMap[K, V]) ComputeIfPresent(key K, remappingFunc func(K, V) V) (V, bool) {
	if oldValue, exists := l.Get(key); exists {
		newValue := remappingFunc(key, oldValue)
		l.Put(key, newValue)
		return newValue, true
	}

	var zero V
	return zero, false
}

// Merge associates the specified value with the specified key if the key is not
// already associated with a value, or merges the existing value with the new value
// using the provided remapping function.
// New mappings are added at the end; existing mappings preserve their position.
func (l *LinkedMap[K, V]) Merge(key K, value V, remappingFunc func(V, V) V) V {
	if oldValue, exists := l.Get(key); exists {
		newValue := remappingFunc(oldValue, value)
		l.Put(key, newValue)
		return newValue
	}

	l.Put(key, value)
	return value
}

// ReplaceAll replaces each entry's value with the result of invoking the given
// function on that entry's key and value.
// The insertion order and keys remain unchanged.
func (l *LinkedMap[K, V]) ReplaceAll(function func(K, V) V) {
	current := l.head
	for current != nil {
		current.value = function(current.key, current.value)
		current = current.next
	}
}

// Additional methods specific to LinkedMap

// Iter returns an iterator that yields key-value pairs in insertion order.
// This is the key advantage of LinkedMap over HashMap.
func (l *LinkedMap[K, V]) Iter() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		current := l.head
		for current != nil {
			if !yield(current.key, current.value) {
				return
			}
			current = current.next
		}
	}
}

// IterKeys returns an iterator that yields keys in insertion order.
func (l *LinkedMap[K, V]) IterKeys() iter.Seq[K] {
	return func(yield func(K) bool) {
		current := l.head
		for current != nil {
			if !yield(current.key) {
				return
			}
			current = current.next
		}
	}
}

// IterValues returns an iterator that yields values in insertion order.
func (l *LinkedMap[K, V]) IterValues() iter.Seq[V] {
	return func(yield func(V) bool) {
		current := l.head
		for current != nil {
			if !yield(current.value) {
				return
			}
			current = current.next
		}
	}
}

// Clone creates an independent copy that preserves insertion order.
// The cloned map will have the same key-value pairs in the same order.
func (l *LinkedMap[K, V]) Clone() Map[K, V] {
	cloned := NewLinkedMap[K, V](l.Size())
	cloned.PutAll(l)
	return cloned
}

// First returns the first key-value pair in insertion order.
// Returns zero values and false if the map is empty.
func (l *LinkedMap[K, V]) First() (K, V, bool) {
	if l.head == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}
	return l.head.key, l.head.value, true
}

// Last returns the last key-value pair in insertion order.
// Returns zero values and false if the map is empty.
func (l *LinkedMap[K, V]) Last() (K, V, bool) {
	if l.tail == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}
	return l.tail.key, l.tail.value, true
}

// RemoveFirst removes and returns the first key-value pair in insertion order.
// Returns zero values and false if the map is empty.
func (l *LinkedMap[K, V]) RemoveFirst() (K, V, bool) {
	if l.head == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}

	key := l.head.key
	value := l.head.value
	l.Remove(key)
	return key, value, true
}

// RemoveLast removes and returns the last key-value pair in insertion order.
// Returns zero values and false if the map is empty.
func (l *LinkedMap[K, V]) RemoveLast() (K, V, bool) {
	if l.tail == nil {
		var zeroK K
		var zeroV V
		return zeroK, zeroV, false
	}

	key := l.tail.key
	value := l.tail.value
	l.Remove(key)
	return key, value, true
}
