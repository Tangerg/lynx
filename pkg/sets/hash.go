package sets

import (
	"iter"
	"maps"
)

// HashSet is a hash table-based Set implementation using Go's built-in map.
// It provides excellent performance with O(1) average case for all basic operations,
// but does not preserve insertion order.
type HashSet[T comparable] map[T]struct{}

// NewHashSet creates a new hash-based set implementation.
// HashSet provides O(1) average time complexity for basic operations
// but does not maintain any particular order of elements.
//
// The optional size parameter can be used to specify the initial capacity
// to avoid map reallocations. If multiple size values are provided,
// only the last positive value is used.
//
// Example:
//
//	set := NewHashSet[int]()           // default capacity
//	set := NewHashSet[int](100)        // initial capacity of 100
//	set := NewHashSet[string](0,50)   // capacity of 50 (last positive value)
func NewHashSet[T comparable](size ...int) HashSet[T] {
	var c = 0
	for _, s := range size {
		if s > 0 {
			c = s
		}
	}
	return make(HashSet[T], c)
}

// Size returns the number of elements with O(1) time complexity.
func (s HashSet[T]) Size() int {
	return len(s)
}

// IsEmpty checks if the set is empty with O(1) time complexity.
func (s HashSet[T]) IsEmpty() bool {
	return s.Size() == 0
}

// Contains checks element existence with O(1) average time complexity.
func (s HashSet[T]) Contains(x T) bool {
	_, ok := s[x]
	return ok
}

// ContainsAll checks if all specified elements exist in the set.
// Short-circuits on the first missing element for efficiency.
func (s HashSet[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if !s.Contains(item) {
			return false
		}
	}
	return true
}

// ContainsAny checks if any of the specified elements exist in the set.
// Short-circuits on the first found element for efficiency.
func (s HashSet[T]) ContainsAny(items ...T) bool {
	for _, item := range items {
		if s.Contains(item) {
			return true
		}
	}
	return false
}

// Add inserts an element with O(1) average time complexity.
// Returns false if the element already exists.
func (s HashSet[T]) Add(x T) bool {
	if s.Contains(x) {
		return false
	}
	s[x] = struct{}{}
	return true
}

// AddAll inserts multiple elements efficiently.
// Returns true if at least one element was actually added.
func (s HashSet[T]) AddAll(items ...T) bool {
	changed := false
	for _, item := range items {
		if s.Add(item) {
			changed = true
		}
	}
	return changed
}

// Remove deletes an element with O(1) average time complexity.
// Returns false if the element doesn't exist.
func (s HashSet[T]) Remove(x T) bool {
	if !s.Contains(x) {
		return false
	}
	delete(s, x)
	return true
}

// RemoveAll deletes multiple elements efficiently.
// Returns true if at least one element was actually removed.
func (s HashSet[T]) RemoveAll(items ...T) bool {
	changed := false
	for _, item := range items {
		if s.Remove(item) {
			changed = true
		}
	}
	return changed
}

// Retain keeps only the specified element, removing all others.
// Returns false if the element doesn't exist and the set is already empty.
func (s HashSet[T]) Retain(x T) bool {
	return s.RetainAll(x)
}

// RetainAll keeps only elements that are present in the items slice.
// If items is empty, clears the entire set.
func (s HashSet[T]) RetainAll(items ...T) bool {
	if len(items) == 0 {
		if s.IsEmpty() {
			return false
		}
		s.Clear()
		return true
	}

	toRetain := make(HashSet[T], len(items))
	for _, item := range items {
		toRetain[item] = struct{}{}
	}

	changed := false
	for key := range s {
		if !toRetain.Contains(key) {
			delete(s, key)
			changed = true
		}
	}

	return changed
}

// Clear removes all elements using Go's built-in clear function.
// This is more efficient than manually deleting each element.
func (s HashSet[T]) Clear() {
	clear(s)
}

// Iter returns an iterator over the set elements in undefined order.
// Uses the efficient maps.Keys function from the standard library.
func (s HashSet[T]) Iter() iter.Seq[T] {
	return maps.Keys(s)
}

// ToSlice returns a slice containing all set elements in undefined order.
// The slice is pre-allocated with the correct capacity for efficiency.
func (s HashSet[T]) ToSlice() []T {
	slice := make([]T, 0, s.Size())
	for x := range s {
		slice = append(slice, x)
	}
	return slice
}

// Clone creates an independent copy of the set.
// The new set has the same capacity as the original for efficiency.
func (s HashSet[T]) Clone() Set[T] {
	result := NewHashSet[T](s.Size())
	for x := range s {
		result.Add(x)
	}
	return result
}
