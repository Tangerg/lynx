package sets

import (
	"iter"
)

type node[T comparable] struct {
	value T
	prev  *node[T]
	next  *node[T]
}

// LinkedSet is a Set implementation that maintains insertion order.
// It combines a hash map for O(1) lookups with a doubly-linked list
// for maintaining and iterating over elements in insertion order.
//
// This implementation provides:
//   - O(1) lookup, insertion, and deletion
//   - Predictable iteration order (insertion order)
//   - Memory overhead for storing linked list pointers
type LinkedSet[T comparable] struct {
	nodes map[T]*node[T] // Maps elements to their linked list nodes for O(1) access
	head  *node[T]       // First element in insertion order
	tail  *node[T]       // Last element in insertion order
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
func NewLinkedSet[T comparable](size ...int) *LinkedSet[T] {
	var c = 0
	for _, s := range size {
		if s > 0 {
			c = s
		}
	}
	ls := &LinkedSet[T]{
		nodes: make(map[T]*node[T], c),
	}
	return ls
}

// Add inserts an element at the end of the insertion order.
// Returns false if the element already exists.
// Time complexity: O(1)
func (l *LinkedSet[T]) Add(x T) bool {
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
func (l *LinkedSet[T]) AddAll(items ...T) bool {
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
func (l *LinkedSet[T]) Remove(x T) bool {
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
func (l *LinkedSet[T]) removeNode(node *node[T]) {
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
func (l *LinkedSet[T]) RemoveAll(items ...T) bool {
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
func (l *LinkedSet[T]) Contains(x T) bool {
	_, exists := l.nodes[x]
	return exists
}

// ContainsAll checks if all specified elements exist in the set.
// Short-circuits on the first missing element.
func (l *LinkedSet[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if !l.Contains(item) {
			return false
		}
	}
	return true
}

// ContainsAny checks if any of the specified elements exist in the set.
// Short-circuits on the first found element.
func (l *LinkedSet[T]) ContainsAny(items ...T) bool {
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
func (l *LinkedSet[T]) Retain(x T) bool {
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
func (l *LinkedSet[T]) RetainAll(items ...T) bool {
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
func (l *LinkedSet[T]) Size() int {
	return len(l.nodes)
}

// IsEmpty checks if the set contains no elements.
// Time complexity: O(1)
func (l *LinkedSet[T]) IsEmpty() bool {
	return l.Size() == 0
}

// Clear removes all elements and resets the linked list structure.
// Carefully clears all node pointers to prevent memory leaks.
func (l *LinkedSet[T]) Clear() {
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
func (l *LinkedSet[T]) Iter() iter.Seq[T] {
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
func (l *LinkedSet[T]) ToSlice() []T {
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
func (l *LinkedSet[T]) Clone() Set[T] {
	cloned := NewLinkedSet[T](l.Size())
	// Add elements in insertion order to preserve order in the clone
	current := l.head
	for current != nil {
		cloned.Add(current.value)
		current = current.next
	}
	return cloned
}
