package sets

import (
	"iter"
	"maps"
	"sync"
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
func NewHashSet[T comparable](size ...int) Set[T] {
	var c = 0
	for _, s := range size {
		if s > 0 {
			c = s
		}
	}
	return make(hashSet[T], c)
}

// hashSet is a hash table-based Set implementation using Go's built-in map.
// It provides excellent performance with O(1) average case for all basic operations,
// but does not preserve insertion order.
//
// The zero value is ready to use, but prefer using NewHashSet for better
// initial capacity management.
type hashSet[T comparable] map[T]struct{}

// Iter returns an iterator over the set elements in undefined order.
// Uses the efficient maps.Keys function from the standard library.
func (s hashSet[T]) Iter() iter.Seq[T] {
	return maps.Keys(s)
}

// ToSlice returns a slice containing all set elements in undefined order.
// The slice is pre-allocated with the correct capacity for efficiency.
func (s hashSet[T]) ToSlice() []T {
	slice := make([]T, 0, s.Size())
	for x := range s {
		slice = append(slice, x)
	}
	return slice
}

// Contains checks element existence with O(1) average time complexity.
func (s hashSet[T]) Contains(x T) bool {
	_, ok := s[x]
	return ok
}

// ContainsAll checks if all specified elements exist in the set.
// Short-circuits on the first missing element for efficiency.
func (s hashSet[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if !s.Contains(item) {
			return false
		}
	}
	return true
}

// ContainsAny checks if any of the specified elements exist in the set.
// Short-circuits on the first found element for efficiency.
func (s hashSet[T]) ContainsAny(items ...T) bool {
	for _, item := range items {
		if s.Contains(item) {
			return true
		}
	}
	return false
}

// Retain keeps only the specified element, removing all others.
// Returns false if the element doesn't exist and the set is already empty.
func (s hashSet[T]) Retain(x T) bool {
	if !s.Contains(x) {
		return false
	}
	s.Clear()
	s[x] = struct{}{}
	return true
}

// RetainAll keeps only elements that are present in the items slice.
// If items is empty, clears the entire set.
// Optimized to minimize map lookups by creating a lookup set for items.
func (s hashSet[T]) RetainAll(items ...T) bool {
	if len(items) == 0 {
		if s.IsEmpty() {
			return false
		}
		s.Clear()
		return true
	}

	toRetain := make(hashSet[T])
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

// Add inserts an element with O(1) average time complexity.
// Returns false if the element already exists.
func (s hashSet[T]) Add(x T) bool {
	if s.Contains(x) {
		return false
	}
	s[x] = struct{}{}
	return true
}

// AddAll inserts multiple elements efficiently.
// Returns true if at least one element was actually added.
func (s hashSet[T]) AddAll(items ...T) bool {
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
func (s hashSet[T]) Remove(x T) bool {
	if !s.Contains(x) {
		return false
	}
	delete(s, x)
	return true
}

// RemoveAll deletes multiple elements efficiently.
// Returns true if at least one element was actually removed.
func (s hashSet[T]) RemoveAll(items ...T) bool {
	changed := false
	for _, item := range items {
		if s.Remove(item) {
			changed = true
		}
	}
	return changed
}

// Size returns the number of elements with O(1) time complexity.
func (s hashSet[T]) Size() int {
	return len(s)
}

// IsEmpty checks if the set is empty with O(1) time complexity.
func (s hashSet[T]) IsEmpty() bool {
	return s.Size() == 0
}

// Clone creates an independent copy of the set.
// The new set has the same capacity as the original for efficiency.
func (s hashSet[T]) Clone() Set[T] {
	result := NewHashSet[T](s.Size())
	for x := range s {
		result.Add(x)
	}
	return result
}

// Clear removes all elements using Go's built-in clear function.
// This is more efficient than manually deleting each element.
func (s hashSet[T]) Clear() {
	clear(s)
}

type node[T comparable] struct {
	value T
	prev  *node[T]
	next  *node[T]
}

// NewLinkedSet creates a new insertion-ordered set implementation.
// LinkedSet maintains the order in which elements were first added,
// providing predictable iteration order while still offering O(1)
// lookup performance through an internal hash map.
//
// The optional size parameter specifies initial capacity for the internal map.
// If multiple size values are provided, only the last positive value is used.
//
// Example:
//
//	set := NewLinkedSet[string]()     // default capacity
//	set := NewLinkedSet[int](50)      // initial capacity of 50
//
// Use cases:
//   - When you need set semantics but want predictable iteration order
//   - Building ordered unique collections
//   - Maintaining insertion history
func NewLinkedSet[T comparable](size ...int) Set[T] {
	var c = 0
	for _, s := range size {
		if s > 0 {
			c = s
		}
	}
	ls := &linkedSet[T]{
		nodes: make(map[T]*node[T], c),
	}
	return ls
}

// linkedSet is a Set implementation that maintains insertion order.
// It combines a hash map for O(1) lookups with a doubly-linked list
// for maintaining and iterating over elements in insertion order.
//
// This implementation provides:
//   - O(1) lookup, insertion, and deletion
//   - Predictable iteration order (insertion order)
//   - Memory overhead for storing linked list pointers
type linkedSet[T comparable] struct {
	nodes map[T]*node[T] // Maps elements to their linked list nodes for O(1) access
	head  *node[T]       // First element in insertion order
	tail  *node[T]       // Last element in insertion order
}

// Add inserts an element at the end of the insertion order.
// Returns false if the element already exists.
// Time complexity: O(1)
func (l *linkedSet[T]) Add(x T) bool {
	// Check existence using the hash map for O(1) lookup
	if l.Contains(x) {
		return false
	}

	// Create new node and add to hash map
	newNode := &node[T]{value: x}
	l.nodes[x] = newNode

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

	return true
}

// AddAll adds multiple elements in the order they appear in the items slice.
// Elements already in the set are skipped, maintaining the original insertion order.
// This method is optimized for batch operations and minimizes linked list traversals.
func (l *linkedSet[T]) AddAll(items ...T) bool {
	if len(items) == 0 {
		return false
	}

	// Pre-filter items to find which ones actually need to be added
	// This creates a slice of only new items, avoiding redundant work
	toAdd := make([]T, 0, len(items))
	for _, item := range items {
		if _, exists := l.nodes[item]; !exists {
			toAdd = append(toAdd, item)
		}
	}

	// If no items need to be added, return early
	if len(toAdd) == 0 {
		return false
	}

	// Batch create all nodes first
	newNodes := make([]*node[T], len(toAdd))
	for i, item := range toAdd {
		newNodes[i] = &node[T]{value: item}
		l.nodes[item] = newNodes[i]
	}

	// Link all nodes to the existing list structure
	if l.tail == nil {
		// Empty list case - chain all new nodes together
		l.head = newNodes[0]
		for i := 0; i < len(newNodes)-1; i++ {
			newNodes[i].next = newNodes[i+1]
			newNodes[i+1].prev = newNodes[i]
		}
		l.tail = newNodes[len(newNodes)-1]
	} else {
		// Non-empty list case - append all new nodes to the end
		firstNew := newNodes[0]
		firstNew.prev = l.tail
		l.tail.next = firstNew

		// Chain the new nodes together
		for i := 0; i < len(newNodes)-1; i++ {
			newNodes[i].next = newNodes[i+1]
			newNodes[i+1].prev = newNodes[i]
		}

		// Update tail to point to the last new node
		l.tail = newNodes[len(newNodes)-1]
	}

	return true
}

// Remove removes an element while maintaining the linked list structure.
// Returns false if the element doesn't exist.
// Time complexity: O(1)
func (l *linkedSet[T]) Remove(x T) bool {
	// Find the node using the hash map for O(1) lookup
	nodeToRemove, exists := l.nodes[x]
	if !exists {
		return false
	}

	// Remove from hash map
	delete(l.nodes, x)

	// Remove from linked list structure
	l.removeNode(nodeToRemove)

	return true
}

// removeNode removes a node from the doubly-linked list and updates head/tail pointers.
// This is a helper method that handles all the pointer manipulation safely.
func (l *linkedSet[T]) removeNode(node *node[T]) {
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

// RemoveAll removes multiple elements efficiently.
// Uses batch processing to minimize linked list traversals.
func (l *linkedSet[T]) RemoveAll(items ...T) bool {
	if len(items) == 0 {
		return false
	}

	// Pre-identify nodes that actually exist to avoid redundant work
	toRemoves := make([]*node[T], 0, len(items))
	for _, item := range items {
		if toRemove, exists := l.nodes[item]; exists {
			toRemoves = append(toRemoves, toRemove)
		}
	}

	if len(toRemoves) == 0 {
		return false
	}

	// Batch remove all identified nodes
	for _, toRemove := range toRemoves {
		delete(l.nodes, toRemove.value)
		l.removeNode(toRemove)
	}

	return true
}

// Contains checks element existence using the internal hash map.
// Time complexity: O(1)
func (l *linkedSet[T]) Contains(x T) bool {
	_, exists := l.nodes[x]
	return exists
}

// ContainsAll checks if all specified elements exist in the set.
// Short-circuits on the first missing element.
func (l *linkedSet[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if !l.Contains(item) {
			return false
		}
	}
	return true
}

// ContainsAny checks if any of the specified elements exist in the set.
// Short-circuits on the first found element.
func (l *linkedSet[T]) ContainsAny(items ...T) bool {
	for _, item := range items {
		if l.Contains(item) {
			return true
		}
	}
	return false
}

// Retain keeps only the specified element, removing all others.
// If the element doesn't exist, the set becomes empty.
// Optimized to detect when no change is needed.
func (l *linkedSet[T]) Retain(x T) bool {
	if !l.Contains(x) {
		if l.IsEmpty() {
			return false // No change needed
		}
		l.Clear()
		return true // Set was cleared
	}

	// If set contains only the element to retain, no change needed
	if l.Size() == 1 {
		return false
	}

	// Remove all elements except the one to retain
	current := l.head
	changed := false

	for current != nil {
		next := current.next // Save next before potential deletion
		if current.value != x {
			delete(l.nodes, current.value)
			l.removeNode(current)
			changed = true
		}
		current = next
	}

	return changed
}

// RetainAll keeps only elements that appear in the items slice.
// This effectively performs a set intersection operation.
// If items is empty, the set is cleared.
func (l *linkedSet[T]) RetainAll(items ...T) bool {
	if len(items) == 0 {
		if l.IsEmpty() {
			return false
		}
		l.Clear()
		return true
	}

	// Create a lookup map for elements to retain
	toRetain := make(map[T]struct{}, len(items))
	for _, item := range items {
		toRetain[item] = struct{}{}
	}

	// Traverse the linked list and remove elements not in the retain set
	current := l.head
	changed := false

	for current != nil {
		next := current.next // Save next before potential deletion
		if _, shouldRetain := toRetain[current.value]; !shouldRetain {
			delete(l.nodes, current.value)
			l.removeNode(current)
			changed = true
		}
		current = next
	}

	return changed
}

// Size returns the number of elements using the hash map size.
// Time complexity: O(1)
func (l *linkedSet[T]) Size() int {
	return len(l.nodes)
}

// IsEmpty checks if the set contains no elements.
// Time complexity: O(1)
func (l *linkedSet[T]) IsEmpty() bool {
	return l.Size() == 0
}

// Clear removes all elements and resets the linked list structure.
// Carefully clears all node pointers to prevent memory leaks.
func (l *linkedSet[T]) Clear() {
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

// Iter returns an iterator that yields elements in insertion order.
// This is the key advantage of LinkedSet over HashSet.
func (l *linkedSet[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		current := l.head
		for current != nil {
			if !yield(current.value) {
				return
			}
			current = current.next
		}
	}
}

// ToSlice returns a slice with elements in insertion order.
// The slice is pre-allocated with the correct capacity for efficiency.
func (l *linkedSet[T]) ToSlice() []T {
	result := make([]T, 0, l.Size())
	current := l.head
	for current != nil {
		result = append(result, current.value)
		current = current.next
	}
	return result
}

// Clone creates an independent copy that preserves insertion order.
// The cloned set will have the same elements in the same order.
func (l *linkedSet[T]) Clone() Set[T] {
	cloned := NewLinkedSet[T](l.Size())
	// Add elements in insertion order to preserve order in the clone
	current := l.head
	for current != nil {
		cloned.Add(current.value)
		current = current.next
	}
	return cloned
}

// syncSet provides a thread-safe wrapper around any Set implementation.
// It uses a read-write mutex to allow concurrent reads while ensuring
// exclusive access for write operations.
//
// The implementation is designed to minimize lock contention:
//   - Read operations (Contains, Size, etc.) use read locks
//   - Write operations (Add, Remove, etc.) use write locks
//   - Iteration creates a snapshot to avoid holding locks during iteration
type syncSet[T comparable] struct {
	inner Set[T]       // The underlying Set implementation
	mutex sync.RWMutex // Read-write mutex for thread safety
}

// NewSyncSet creates a new thread-safe set wrapper.
// If a Set is provided, it wraps that set; otherwise, it creates a new HashSet.
// If the provided set is already a SyncSet, it returns the same instance
// to avoid double-wrapping.
//
// Examples:
//
//	syncSet := NewSyncSet[int]()                    // wraps a new HashSet
//	syncSet := NewSyncSet(NewLinkedSet[string]())   // wraps a LinkedSet
//	syncSet := NewSyncSet(existingSyncSet)          // returns existingSyncSet
//
// The wrapped set should not be accessed directly after wrapping to maintain
// thread safety guarantees.
func NewSyncSet[T comparable](sets ...Set[T]) Set[T] {
	var inner = NewHashSet[T]()

	for _, s := range sets {
		if s != nil {
			// Avoid double-wrapping if the provided set is already thread-safe
			if ss, ok := s.(*syncSet[T]); ok {
				return ss
			}
			inner = s.Clone()
			break
		}
	}

	return &syncSet[T]{
		inner: inner,
	}
}

// Add safely adds an element with exclusive access.
// Uses a write lock to ensure thread safety.
func (s *syncSet[T]) Add(x T) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Add(x)
}

// AddAll safely adds multiple elements with exclusive access.
// The entire operation is atomic - either all applicable elements
// are added or none are (in case of panic).
func (s *syncSet[T]) AddAll(items ...T) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.AddAll(items...)
}

// Remove safely removes an element with exclusive access.
// Uses a write lock to ensure thread safety.
func (s *syncSet[T]) Remove(x T) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Remove(x)
}

// RemoveAll safely removes multiple elements with exclusive access.
// The entire operation is atomic.
func (s *syncSet[T]) RemoveAll(items ...T) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.RemoveAll(items...)
}

// Contains safely checks element existence with shared access.
// Uses a read lock to allow concurrent reads while preventing
// concurrent writes.
func (s *syncSet[T]) Contains(x T) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Contains(x)
}

// ContainsAll safely checks multiple elements with shared access.
// Uses a read lock for the entire operation.
func (s *syncSet[T]) ContainsAll(items ...T) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.ContainsAll(items...)
}

// ContainsAny safely checks for any matching elements with shared access.
// Uses a read lock for the entire operation.
func (s *syncSet[T]) ContainsAny(items ...T) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.ContainsAny(items...)
}

// Retain safely keeps only the specified element with exclusive access.
// Uses a write lock as this is a mutating operation.
func (s *syncSet[T]) Retain(x T) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.Retain(x)
}

// RetainAll safely retains only specified elements with exclusive access.
// The entire operation is atomic.
func (s *syncSet[T]) RetainAll(items ...T) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.inner.RetainAll(items...)
}

// Size safely returns the element count with shared access.
// Uses a read lock to ensure a consistent view.
func (s *syncSet[T]) Size() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.Size()
}

// IsEmpty safely checks if the set is empty with shared access.
// Uses a read lock to ensure a consistent view.
func (s *syncSet[T]) IsEmpty() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.IsEmpty()
}

// Clear safely removes all elements with exclusive access.
// Uses a write lock as this is a mutating operation.
func (s *syncSet[T]) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.inner.Clear()
}

// Iter returns a thread-safe iterator by creating a snapshot.
// This approach avoids holding locks during iteration, which could
// cause deadlocks or performance issues with long-running iterations.
//
// The snapshot is taken at the moment Iter() is called, so changes
// made to the set during iteration won't be reflected in the iteration.
// This provides a consistent view but means the iteration might not
// reflect the current state of the set.
//
// Example:
//
//	for element := range syncSet.Iter() {
//	    // This iteration is safe and won't block other operations
//	    fmt.Println(element)
//	}
func (s *syncSet[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		// Create a snapshot with minimal lock time
		s.mutex.RLock()
		snapshot := s.inner.ToSlice()
		s.mutex.RUnlock()

		// Iterate over the snapshot without holding any locks
		for _, item := range snapshot {
			if !yield(item) {
				return
			}
		}
	}
}

// ToSlice safely returns a snapshot of all elements.
// Uses a read lock to ensure a consistent view.
// The returned slice is independent of the set.
func (s *syncSet[T]) ToSlice() []T {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.inner.ToSlice()
}

// Clone safely creates an independent copy of the set.
// The clone is also a SyncSet wrapping a copy of the inner set.
// Uses a read lock to ensure a consistent snapshot for cloning.
func (s *syncSet[T]) Clone() Set[T] {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	cloned := s.inner.Clone()
	return &syncSet[T]{
		inner: cloned,
	}
}
