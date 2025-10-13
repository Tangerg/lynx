package sets

import (
	"iter"
)

// Set represents a collection that contains no duplicate elements. More formally,
// sets contain no pair of elements e1 and e2 such that e1 == e2.
// As implied by its name, this interface models the mathematical set abstraction.
//
// The Set interface places additional requirements on the contracts of all
// constructors and methods. All constructors must create a set that contains
// no duplicate elements.
//
// Great care must be exercised if mutable objects are used as set elements.
// The behavior of a set is not specified if the value of an object is changed
// in a manner that affects equality comparisons while the object is an element
// in the set.
//
// This interface provides three main implementations:
//   - HashSet: Hash table based implementation with O(1) average performance
//   - LinkedSet: Hash table + doubly-linked list maintaining insertion order
//   - SyncSet: Thread-safe wrapper using read-write mutex
//
// All Set implementations work with any comparable type, providing compile-time
// type safety.
//
// Basic usage:
//
//	set := NewHashSet[string]()
//	changed := set.Add("hello")     // returns true
//	changed = set.Add("hello")      // returns false (already exists)
//	exists := set.Contains("hello") // returns true
//	size := set.Size()              // returns 1
type Set[T comparable] interface {
	// Size returns the number of elements in this set (its cardinality).
	Size() int

	// IsEmpty returns true if this set contains no elements.
	IsEmpty() bool

	// Contains returns true if this set contains the specified element.
	// More formally, returns true if and only if this set contains an element e
	// such that e == x.
	Contains(x T) bool

	// ContainsAll returns true if this set contains all of the specified elements.
	// Returns true for an empty argument list.
	ContainsAll(items ...T) bool

	// ContainsAny returns true if this set contains any of the specified elements.
	// Returns false for an empty argument list.
	ContainsAny(items ...T) bool

	// Add adds the specified element to this set if it is not already present.
	// Returns true if this set did not already contain the specified element.
	// If this set already contains the element, the call leaves the set unchanged
	// and returns false. This ensures that sets never contain duplicate elements.
	Add(x T) bool

	// AddAll adds all of the specified elements to this set if they're not already present.
	// This operation effectively modifies this set so that its value is the union
	// of the original set and the specified elements.
	// Returns true if this set changed as a result of the call.
	AddAll(items ...T) bool

	// Remove removes the specified element from this set if it is present.
	// Returns true if this set contained the element (or equivalently, if this set
	// changed as a result of the call). The set will not contain the element once
	// the call returns.
	Remove(x T) bool

	// RemoveAll removes all of the specified elements from this set if they are present.
	// This operation effectively modifies this set so that its value is the asymmetric
	// set difference of the original set and the specified elements.
	// Returns true if this set changed as a result of the call.
	RemoveAll(items ...T) bool

	// Retain retains only the specified element in this set, removing all others.
	// After this operation, the set will contain only the specified element
	// (if it was present), or be empty (if it wasn't present).
	// Returns true if this set changed as a result of the call.
	Retain(x T) bool

	// RetainAll retains only the elements in this set that are contained in the
	// specified items. In other words, removes from this set all of its elements
	// that are not contained in the specified items. This operation effectively
	// modifies this set so that its value is the intersection of the two sets.
	// Returns true if this set changed as a result of the call.
	// If items is empty, this method clears the set.
	RetainAll(items ...T) bool

	// Clear removes all of the elements from this set.
	// The set will be empty after this call returns.
	Clear()

	// Iter returns an iterator over the elements in this set.
	// The elements are returned in no particular order unless this set is an
	// instance of an implementation that provides ordering guarantees
	// (such as LinkedSet which maintains insertion order).
	//
	// The iterator is designed to work with Go's range-over-function:
	//
	//	for element := range set.Iter() {
	//		// process element
	//	}
	//
	// The iteration behavior depends on the Set implementation:
	//   - HashSet: undefined order
	//   - LinkedSet: insertion order
	//   - SyncSet: depends on underlying implementation (uses snapshot for thread safety)
	Iter() iter.Seq[T]

	// ToSlice returns a slice containing all of the elements in this set.
	// If this set makes any guarantees as to what order its elements are returned
	// by its iterator, this method must return the elements in the same order.
	//
	// The returned slice is "safe" in that no references to it are maintained
	// by this set. The caller is free to modify the returned slice.
	//
	// This method acts as a bridge between set-based and slice-based APIs.
	ToSlice() []T

	// Clone creates a shallow copy of this set.
	// The returned set is independent of the original and can be modified
	// without affecting the original set. The elements themselves are not cloned.
	Clone() Set[T]
}
