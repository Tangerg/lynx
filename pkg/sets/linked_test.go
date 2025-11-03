package sets

import (
	"fmt"
	"testing"
)

// ============================================================================
// Constructor Tests
// ============================================================================

func TestNewLinkedSet(t *testing.T) {
	t.Run("create with default capacity", func(t *testing.T) {
		set := NewLinkedSet[int]()
		if set == nil {
			t.Error("NewLinkedSet() returned nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
		if set.head != nil {
			t.Error("head should be nil for empty set")
		}
		if set.tail != nil {
			t.Error("tail should be nil for empty set")
		}
	})

	t.Run("create with specific capacity", func(t *testing.T) {
		set := NewLinkedSet[string](100)
		if set == nil {
			t.Error("NewLinkedSet(100) returned nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("create with multiple size parameters", func(t *testing.T) {
		set := NewLinkedSet[int](10, 0, 50, -1, 30)
		if set == nil {
			t.Error("NewLinkedSet() with multiple params returned nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("create with zero and negative sizes", func(t *testing.T) {
		set := NewLinkedSet[int](0, -10, -5)
		if set == nil {
			t.Error("NewLinkedSet() with non-positive sizes returned nil")
		}
	})
}

// ============================================================================
// Add Tests
// ============================================================================

func TestLinkedSet_Add(t *testing.T) {
	t.Run("add single element to empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.Add(1)

		if !changed {
			t.Error("Add(1) = false, want true")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(1) {
			t.Error("Contains(1) = false, want true")
		}
		if set.head == nil || set.head.value != 1 {
			t.Error("head should point to element 1")
		}
		if set.tail == nil || set.tail.value != 1 {
			t.Error("tail should point to element 1")
		}
		if set.head != set.tail {
			t.Error("head and tail should be same for single element")
		}
	})

	t.Run("add duplicate element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(1)
		changed := set.Add(1)

		if changed {
			t.Error("Add(1) duplicate = true, want false")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
	})

	t.Run("add multiple different elements", func(t *testing.T) {
		set := NewLinkedSet[string]()
		elements := []string{"apple", "banana", "cherry"}

		for i, elem := range elements {
			if !set.Add(elem) {
				t.Errorf("Add(%v) = false, want true", elem)
			}
			if set.Size() != i+1 {
				t.Errorf("Size() = %v, want %v", set.Size(), i+1)
			}
		}
	})

	t.Run("add maintains insertion order", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(3)
		set.Add(1)
		set.Add(4)

		slice := set.ToSlice()
		expected := []int{3, 1, 4}

		if len(slice) != len(expected) {
			t.Errorf("len(slice) = %v, want %v", len(slice), len(expected))
		}

		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("add zero value", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.Add(0)

		if !changed {
			t.Error("Add(0) = false, want true")
		}
		if !set.Contains(0) {
			t.Error("Contains(0) = false, want true")
		}
	})

	t.Run("add verifies linked list structure", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(1)
		set.Add(2)
		set.Add(3)

		// Verify forward links
		if set.head.value != 1 {
			t.Error("head should be 1")
		}
		if set.head.next.value != 2 {
			t.Error("head.next should be 2")
		}
		if set.head.next.next.value != 3 {
			t.Error("head.next.next should be 3")
		}
		if set.tail.value != 3 {
			t.Error("tail should be 3")
		}

		// Verify backward links
		if set.tail.prev.value != 2 {
			t.Error("tail.prev should be 2")
		}
		if set.tail.prev.prev.value != 1 {
			t.Error("tail.prev.prev should be 1")
		}
	})
}

// ============================================================================
// AddAll Tests
// ============================================================================

func TestLinkedSet_AddAll(t *testing.T) {
	t.Run("add empty list", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.AddAll()

		if changed {
			t.Error("AddAll() with no args = true, want false")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("add multiple unique elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.AddAll(1, 2, 3, 4, 5)

		if !changed {
			t.Error("AddAll(1,2,3,4,5) = false, want true")
		}
		if set.Size() != 5 {
			t.Errorf("Size() = %v, want 5", set.Size())
		}

		// Verify order
		slice := set.ToSlice()
		for i := 0; i < 5; i++ {
			if slice[i] != i+1 {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], i+1)
			}
		}
	})

	t.Run("add with duplicates in arguments", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.AddAll(1, 2, 2, 3, 3, 3)

		if !changed {
			t.Error("AddAll() = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}

		// First occurrence should be kept
		slice := set.ToSlice()
		expected := []int{1, 2, 3}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("add to non-empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.AddAll(3, 4, 5)

		if !changed {
			t.Error("AddAll(3,4,5) = false, want true")
		}
		if set.Size() != 5 {
			t.Errorf("Size() = %v, want 5", set.Size())
		}

		// Verify order: 1,2,3 then 4,5 (3 already exists)
		slice := set.ToSlice()
		expected := []int{1, 2, 3, 4, 5}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("add all existing elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.AddAll(1, 2, 3)

		if changed {
			t.Error("AddAll() with all existing = true, want false")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("add all maintains linked list integrity", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		// Verify forward traversal
		current := set.head
		for i := 1; i <= 5; i++ {
			if current == nil {
				t.Fatalf("Unexpected nil at position %v", i)
			}
			if current.value != i {
				t.Errorf("Position %v: value = %v, want %v", i, current.value, i)
			}
			current = current.next
		}
		if current != nil {
			t.Error("Expected nil after last element")
		}

		// Verify backward traversal
		current = set.tail
		for i := 5; i >= 1; i-- {
			if current == nil {
				t.Fatalf("Unexpected nil at position %v", i)
			}
			if current.value != i {
				t.Errorf("Position %v: value = %v, want %v", i, current.value, i)
			}
			current = current.prev
		}
		if current != nil {
			t.Error("Expected nil before first element")
		}
	})

	t.Run("add all to empty set", func(t *testing.T) {
		set := NewLinkedSet[string]()
		changed := set.AddAll("alpha", "beta", "gamma")

		if !changed {
			t.Error("AddAll() = false, want true")
		}
		if set.head.value != "alpha" {
			t.Error("head should be alpha")
		}
		if set.tail.value != "gamma" {
			t.Error("tail should be gamma")
		}
	})
}

// ============================================================================
// Remove Tests
// ============================================================================

func TestLinkedSet_Remove(t *testing.T) {
	t.Run("remove existing element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(1)
		changed := set.Remove(1)

		if !changed {
			t.Error("Remove(1) = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
		if set.Contains(1) {
			t.Error("Contains(1) after Remove = true, want false")
		}
		if set.head != nil {
			t.Error("head should be nil after removing only element")
		}
		if set.tail != nil {
			t.Error("tail should be nil after removing only element")
		}
	})

	t.Run("remove non-existing element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(1)
		changed := set.Remove(2)

		if changed {
			t.Error("Remove(2) non-existing = true, want false")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
	})

	t.Run("remove from empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.Remove(1)

		if changed {
			t.Error("Remove(1) from empty = true, want false")
		}
	})

	t.Run("remove head element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.Remove(1)

		if !changed {
			t.Error("Remove(1) = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
		if set.head.value != 2 {
			t.Errorf("head.value = %v, want 2", set.head.value)
		}
		if set.head.prev != nil {
			t.Error("head.prev should be nil")
		}
	})

	t.Run("remove tail element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.Remove(3)

		if !changed {
			t.Error("Remove(3) = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
		if set.tail.value != 2 {
			t.Errorf("tail.value = %v, want 2", set.tail.value)
		}
		if set.tail.next != nil {
			t.Error("tail.next should be nil")
		}
	})

	t.Run("remove middle element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.Remove(3)

		if !changed {
			t.Error("Remove(3) = false, want true")
		}
		if set.Size() != 4 {
			t.Errorf("Size() = %v, want 4", set.Size())
		}

		slice := set.ToSlice()
		expected := []int{1, 2, 4, 5}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("remove maintains linked list integrity", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Remove(3)

		// Verify forward links
		current := set.head
		expected := []int{1, 2, 4, 5}
		for i, exp := range expected {
			if current == nil {
				t.Fatalf("Unexpected nil at position %v", i)
			}
			if current.value != exp {
				t.Errorf("Position %v: value = %v, want %v", i, current.value, exp)
			}
			current = current.next
		}

		// Verify backward links
		current = set.tail
		for i := len(expected) - 1; i >= 0; i-- {
			if current == nil {
				t.Fatalf("Unexpected nil at position %v", i)
			}
			if current.value != expected[i] {
				t.Errorf("Position %v: value = %v, want %v", i, current.value, expected[i])
			}
			current = current.prev
		}
	})

	t.Run("remove zero value", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(0)
		changed := set.Remove(0)

		if !changed {
			t.Error("Remove(0) = false, want true")
		}
		if set.Contains(0) {
			t.Error("Contains(0) after Remove = true, want false")
		}
	})
}

// ============================================================================
// RemoveAll Tests
// ============================================================================

func TestLinkedSet_RemoveAll(t *testing.T) {
	t.Run("remove empty list", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll()

		if changed {
			t.Error("RemoveAll() with no args = true, want false")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("remove multiple existing elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RemoveAll(2, 3, 4)

		if !changed {
			t.Error("RemoveAll(2,3,4) = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}

		slice := set.ToSlice()
		expected := []int{1, 5}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("remove non-existing elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll(4, 5, 6)

		if changed {
			t.Error("RemoveAll() non-existing = true, want false")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("remove mix of existing and non-existing", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll(2, 4, 5)

		if !changed {
			t.Error("RemoveAll() mixed = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
		if set.Contains(2) {
			t.Error("Should not contain 2")
		}
	})

	t.Run("remove all elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll(1, 2, 3)

		if !changed {
			t.Error("RemoveAll() all = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
		if set.head != nil || set.tail != nil {
			t.Error("head and tail should be nil")
		}
	})

	t.Run("remove maintains order", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5, 6, 7)
		set.RemoveAll(2, 4, 6)

		slice := set.ToSlice()
		expected := []int{1, 3, 5, 7}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})
}

// ============================================================================
// Contains Tests
// ============================================================================

func TestLinkedSet_Contains(t *testing.T) {
	t.Run("contains existing element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(42)

		if !set.Contains(42) {
			t.Error("Contains(42) = false, want true")
		}
	})

	t.Run("contains non-existing element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(42)

		if set.Contains(99) {
			t.Error("Contains(99) = true, want false")
		}
	})

	t.Run("contains in empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()

		if set.Contains(1) {
			t.Error("Contains(1) in empty set = true, want false")
		}
	})

	t.Run("contains zero value", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(0)

		if !set.Contains(0) {
			t.Error("Contains(0) = false, want true")
		}
	})
}

// ============================================================================
// ContainsAll Tests
// ============================================================================

func TestLinkedSet_ContainsAll(t *testing.T) {
	t.Run("contains all with empty arguments", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if !set.ContainsAll() {
			t.Error("ContainsAll() with no args = false, want true")
		}
	})

	t.Run("contains all existing elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		if !set.ContainsAll(2, 3, 4) {
			t.Error("ContainsAll(2,3,4) = false, want true")
		}
	})

	t.Run("contains all with one missing", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if set.ContainsAll(1, 2, 4) {
			t.Error("ContainsAll(1,2,4) = true, want false")
		}
	})

	t.Run("contains all in empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()

		if set.ContainsAll(1, 2) {
			t.Error("ContainsAll(1,2) in empty = true, want false")
		}
	})

	t.Run("contains all with duplicates in arguments", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if !set.ContainsAll(1, 1, 2, 2, 3) {
			t.Error("ContainsAll() with duplicates = false, want true")
		}
	})
}

// ============================================================================
// ContainsAny Tests
// ============================================================================

func TestLinkedSet_ContainsAny(t *testing.T) {
	t.Run("contains any with empty arguments", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if set.ContainsAny() {
			t.Error("ContainsAny() with no args = true, want false")
		}
	})

	t.Run("contains any existing element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if !set.ContainsAny(3, 4, 5) {
			t.Error("ContainsAny(3,4,5) = false, want true")
		}
	})

	t.Run("contains any with no matches", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if set.ContainsAny(4, 5, 6) {
			t.Error("ContainsAny(4,5,6) = true, want false")
		}
	})

	t.Run("contains any in empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()

		if set.ContainsAny(1, 2, 3) {
			t.Error("ContainsAny(1,2,3) in empty = true, want false")
		}
	})

	t.Run("contains any first element match", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		if !set.ContainsAny(1, 99, 98) {
			t.Error("ContainsAny(1,99,98) = false, want true")
		}
	})
}

// ============================================================================
// Retain Tests
// ============================================================================

func TestLinkedSet_Retain(t *testing.T) {
	t.Run("retain existing element from multiple", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.Retain(3)

		if !changed {
			t.Error("Retain(3) = false, want true")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(3) {
			t.Error("Set should contain only 3")
		}
		if set.head.value != 3 {
			t.Error("head should be 3")
		}
		if set.tail.value != 3 {
			t.Error("tail should be 3")
		}
		if set.head != set.tail {
			t.Error("head and tail should be same")
		}
	})

	t.Run("retain only element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(42)
		changed := set.Retain(42)

		if changed {
			t.Error("Retain(42) single element = true, want false")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(42) {
			t.Error("Set should still contain 42")
		}
	})

	t.Run("retain non-existing from non-empty", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.Retain(99)

		if !changed {
			t.Error("Retain(99) non-existing = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
		if set.head != nil || set.tail != nil {
			t.Error("head and tail should be nil")
		}
	})

	t.Run("retain from empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.Retain(1)

		if changed {
			t.Error("Retain(1) from empty = true, want false")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("retain head element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(10, 20, 30)
		changed := set.Retain(10)

		if !changed {
			t.Error("Retain(10) = false, want true")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(10) {
			t.Error("Should contain 10")
		}
	})

	t.Run("retain tail element", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(10, 20, 30)
		changed := set.Retain(30)

		if !changed {
			t.Error("Retain(30) = false, want true")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(30) {
			t.Error("Should contain 30")
		}
	})

	t.Run("retain verifies linked list structure", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Retain(3)

		if set.head.prev != nil {
			t.Error("head.prev should be nil")
		}
		if set.head.next != nil {
			t.Error("head.next should be nil")
		}
		if set.tail.prev != nil {
			t.Error("tail.prev should be nil")
		}
		if set.tail.next != nil {
			t.Error("tail.next should be nil")
		}
	})
}

// ============================================================================
// RetainAll Tests
// ============================================================================

func TestLinkedSet_RetainAll(t *testing.T) {
	t.Run("retain all with empty arguments", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RetainAll()

		if !changed {
			t.Error("RetainAll() empty args from non-empty = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("retain all with empty arguments on empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		changed := set.RetainAll()

		if changed {
			t.Error("RetainAll() empty args on empty = true, want false")
		}
	})

	t.Run("retain subset", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RetainAll(2, 3, 4)

		if !changed {
			t.Error("RetainAll(2,3,4) = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}

		slice := set.ToSlice()
		expected := []int{2, 3, 4}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("retain with no intersection", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RetainAll(4, 5, 6)

		if !changed {
			t.Error("RetainAll() no intersection = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("retain all elements", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RetainAll(1, 2, 3, 4, 5)

		if changed {
			t.Error("RetainAll() with all elements = true, want false")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("retain with duplicates in arguments", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RetainAll(2, 2, 3, 3, 4, 4)

		if !changed {
			t.Error("RetainAll() with duplicates = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("retain maintains order", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5, 6, 7)
		set.RetainAll(7, 3, 5, 1) // Different order

		// Should maintain original insertion order
		slice := set.ToSlice()
		expected := []int{1, 3, 5, 7}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("retain partial at boundaries", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RetainAll(1, 5)

		if !changed {
			t.Error("RetainAll(1,5) = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}

		slice := set.ToSlice()
		if len(slice) != 2 || slice[0] != 1 || slice[1] != 5 {
			t.Errorf("ToSlice() = %v, want [1 5]", slice)
		}
	})
}

// ============================================================================
// Size and IsEmpty Tests
// ============================================================================

func TestLinkedSet_Size(t *testing.T) {
	t.Run("size of empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()

		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("size after additions", func(t *testing.T) {
		set := NewLinkedSet[int]()

		for i := 1; i <= 10; i++ {
			set.Add(i)
			if set.Size() != i {
				t.Errorf("Size() = %v, want %v", set.Size(), i)
			}
		}
	})

	t.Run("size after removals", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		set.Remove(3)
		if set.Size() != 4 {
			t.Errorf("Size() = %v, want 4", set.Size())
		}

		set.Remove(1)
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("size after clear", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()

		if set.Size() != 0 {
			t.Errorf("Size() after Clear = %v, want 0", set.Size())
		}
	})
}

func TestLinkedSet_IsEmpty(t *testing.T) {
	t.Run("new set is empty", func(t *testing.T) {
		set := NewLinkedSet[int]()

		if !set.IsEmpty() {
			t.Error("IsEmpty() = false, want true")
		}
	})

	t.Run("non-empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(1)

		if set.IsEmpty() {
			t.Error("IsEmpty() = true, want false")
		}
	})

	t.Run("empty after clear", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()

		if !set.IsEmpty() {
			t.Error("IsEmpty() after Clear = false, want true")
		}
	})

	t.Run("empty after removing all", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Add(1)
		set.Remove(1)

		if !set.IsEmpty() {
			t.Error("IsEmpty() = false, want true")
		}
	})
}

// ============================================================================
// Clear Tests
// ============================================================================

func TestLinkedSet_Clear(t *testing.T) {
	t.Run("clear non-empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Clear()

		if set.Size() != 0 {
			t.Errorf("Size() after Clear = %v, want 0", set.Size())
		}
		if !set.IsEmpty() {
			t.Error("IsEmpty() after Clear = false, want true")
		}
		if set.head != nil {
			t.Error("head should be nil after Clear")
		}
		if set.tail != nil {
			t.Error("tail should be nil after Clear")
		}
		if len(set.nodes) != 0 {
			t.Error("nodes map should be empty after Clear")
		}
	})

	t.Run("clear empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.Clear()

		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("clear and reuse", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()
		set.AddAll(4, 5, 6)

		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}

		slice := set.ToSlice()
		expected := []int{4, 5, 6}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("clear multiple times", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)

		set.Clear()
		set.Clear()
		set.Clear()

		if set.Size() != 0 {
			t.Error("Multiple Clear() should keep set empty")
		}
	})
}

// ============================================================================
// Iter Tests
// ============================================================================

func TestLinkedSet_Iter(t *testing.T) {
	t.Run("iterate over empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		count := 0

		for range set.Iter() {
			count++
		}

		if count != 0 {
			t.Errorf("Iterated %v times, want 0", count)
		}
	})

	t.Run("iterate over non-empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		expected := []int{1, 2, 3, 4, 5}
		set.AddAll(expected...)

		count := 0
		for elem := range set.Iter() {
			if elem != expected[count] {
				t.Errorf("Element: got %v, want %v", elem, expected[count])
			}
			count++
		}

		if count != 5 {
			t.Errorf("Iterated %v times, want 5", count)
		}
	})

	t.Run("iterate maintains insertion order", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(5, 3, 8, 1, 9)

		slice := []int{}
		for elem := range set.Iter() {
			slice = append(slice, elem)
		}

		expected := []int{5, 3, 8, 1, 9}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("iterate with break", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		count := 0
		for range set.Iter() {
			count++
			if count == 3 {
				break
			}
		}

		if count != 3 {
			t.Errorf("Iterated %v times, want 3", count)
		}
	})

	t.Run("iterate after modifications", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Remove(3)
		set.Add(6)

		expected := []int{1, 2, 4, 5, 6}
		slice := []int{}
		for elem := range set.Iter() {
			slice = append(slice, elem)
		}

		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})
}

// ============================================================================
// ToSlice Tests
// ============================================================================

func TestLinkedSet_ToSlice(t *testing.T) {
	t.Run("to slice from empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		slice := set.ToSlice()

		if len(slice) != 0 {
			t.Errorf("len(slice) = %v, want 0", len(slice))
		}
	})

	t.Run("to slice from non-empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		slice := set.ToSlice()

		if len(slice) != 5 {
			t.Errorf("len(slice) = %v, want 5", len(slice))
		}

		for i := 0; i < 5; i++ {
			if slice[i] != i+1 {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], i+1)
			}
		}
	})

	t.Run("to slice maintains insertion order", func(t *testing.T) {
		set := NewLinkedSet[string]()
		set.AddAll("charlie", "alpha", "bravo")
		slice := set.ToSlice()

		expected := []string{"charlie", "alpha", "bravo"}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("to slice independence", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		slice := set.ToSlice()

		slice[0] = 999

		if set.Contains(999) {
			t.Error("Modifying slice affected set")
		}
		if !set.Contains(1) {
			t.Error("Original element missing")
		}
	})

	t.Run("to slice after modifications", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Remove(2)
		set.Remove(4)
		slice := set.ToSlice()

		expected := []int{1, 3, 5}
		if len(slice) != len(expected) {
			t.Errorf("len(slice) = %v, want %v", len(slice), len(expected))
		}

		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})
}

// ============================================================================
// Clone Tests
// ============================================================================

func TestLinkedSet_Clone(t *testing.T) {
	t.Run("clone empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		clone := set.Clone()

		if clone.Size() != 0 {
			t.Errorf("Clone size = %v, want 0", clone.Size())
		}
	})

	t.Run("clone non-empty set", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		clone := set.Clone()

		if clone.Size() != set.Size() {
			t.Errorf("Clone size = %v, want %v", clone.Size(), set.Size())
		}

		for i := 1; i <= 5; i++ {
			if !clone.Contains(i) {
				t.Errorf("Clone missing element %v", i)
			}
		}
	})

	t.Run("clone preserves order", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(5, 3, 8, 1, 9)
		clone := set.Clone()

		originalSlice := set.ToSlice()
		cloneSlice := clone.ToSlice()

		for i := range originalSlice {
			if originalSlice[i] != cloneSlice[i] {
				t.Errorf("Position %v: clone[%v] = %v, want %v",
					i, i, cloneSlice[i], originalSlice[i])
			}
		}
	})

	t.Run("clone independence", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		clone := set.Clone()

		clone.Add(4)
		if set.Contains(4) {
			t.Error("Modifying clone affected original")
		}

		set.Add(5)
		if clone.Contains(5) {
			t.Error("Modifying original affected clone")
		}

		set.Remove(1)
		if !clone.Contains(1) {
			t.Error("Removing from original affected clone")
		}
	})

	t.Run("clone type assertion", func(t *testing.T) {
		set := NewLinkedSet[string]()
		set.AddAll("a", "b", "c")
		clone := set.Clone()

		linkedClone, ok := clone.(*LinkedSet[string])
		if !ok {
			t.Error("Clone should be *LinkedSet type")
		}

		if linkedClone == set {
			t.Error("Clone should be a different instance")
		}
	})
}

// ============================================================================
// Different Types Tests
// ============================================================================

func TestLinkedSet_StringType(t *testing.T) {
	t.Run("string set operations", func(t *testing.T) {
		set := NewLinkedSet[string]()
		set.AddAll("apple", "banana", "cherry")

		if !set.Contains("banana") {
			t.Error("Contains(banana) = false, want true")
		}

		set.Remove("banana")
		if set.Contains("banana") {
			t.Error("Contains(banana) after Remove = true, want false")
		}

		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}

		slice := set.ToSlice()
		expected := []string{"apple", "cherry"}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})
}

func TestLinkedSet_StructType(t *testing.T) {
	type Person struct {
		ID   int
		Name string
	}

	t.Run("struct set operations", func(t *testing.T) {
		set := NewLinkedSet[Person]()
		p1 := Person{ID: 1, Name: "Alice"}
		p2 := Person{ID: 2, Name: "Bob"}
		p3 := Person{ID: 3, Name: "Charlie"}

		set.Add(p1)
		set.Add(p2)
		set.Add(p3)

		if !set.Contains(p1) {
			t.Error("Contains(p1) = false, want true")
		}

		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}

		slice := set.ToSlice()
		if len(slice) != 3 {
			t.Errorf("len(slice) = %v, want 3", len(slice))
		}

		if slice[0] != p1 || slice[1] != p2 || slice[2] != p3 {
			t.Error("Order not preserved for struct elements")
		}
	})
}

// ============================================================================
// Edge Cases and Complex Scenarios
// ============================================================================

func TestLinkedSet_ComplexScenarios(t *testing.T) {
	t.Run("alternating add and remove", func(t *testing.T) {
		set := NewLinkedSet[int]()

		for i := 0; i < 100; i++ {
			set.Add(i)
			if i > 0 {
				set.Remove(i - 1)
			}
		}

		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(99) {
			t.Error("Should contain only 99")
		}
	})

	t.Run("large dataset operations", func(t *testing.T) {
		set := NewLinkedSet[int](10000)
		const n = 10000

		for i := 0; i < n; i++ {
			set.Add(i)
		}

		if set.Size() != n {
			t.Errorf("Size() = %v, want %v", set.Size(), n)
		}

		for i := 0; i < n; i += 2 {
			set.Remove(i)
		}

		if set.Size() != n/2 {
			t.Errorf("Size() after removals = %v, want %v", set.Size(), n/2)
		}

		for i := 1; i < n; i += 2 {
			if !set.Contains(i) {
				t.Errorf("Contains(%v) = false, want true", i)
			}
		}
	})

	t.Run("order preservation with duplicates", func(t *testing.T) {
		set := NewLinkedSet[int]()
		set.AddAll(1, 2, 3)
		set.Add(2) // Duplicate
		set.AddAll(4, 5)
		set.Add(1) // Duplicate

		slice := set.ToSlice()
		expected := []int{1, 2, 3, 4, 5}

		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("retain all with complex patterns", func(t *testing.T) {
		set := NewLinkedSet[int]()
		for i := 1; i <= 20; i++ {
			set.Add(i)
		}

		// Retain only even numbers
		evens := []int{}
		for i := 2; i <= 20; i += 2 {
			evens = append(evens, i)
		}
		set.RetainAll(evens...)

		if set.Size() != 10 {
			t.Errorf("Size() = %v, want 10", set.Size())
		}

		for _, even := range evens {
			if !set.Contains(even) {
				t.Errorf("Should contain %v", even)
			}
		}
	})

	t.Run("stress test linked list integrity", func(t *testing.T) {
		set := NewLinkedSet[int]()

		// Add many elements
		for i := 0; i < 1000; i++ {
			set.Add(i)
		}

		// Remove in various patterns
		for i := 0; i < 1000; i += 3 {
			set.Remove(i)
		}

		// Verify linked list forward traversal
		current := set.head
		count := 0
		for current != nil {
			count++
			if current.next != nil && current.next.prev != current {
				t.Error("Linked list integrity broken: forward-backward mismatch")
			}
			current = current.next
		}

		if count != set.Size() {
			t.Errorf("Traversal count %v != Size %v", count, set.Size())
		}

		// Verify linked list backward traversal
		current = set.tail
		count = 0
		for current != nil {
			count++
			if current.prev != nil && current.prev.next != current {
				t.Error("Linked list integrity broken: backward-forward mismatch")
			}
			current = current.prev
		}

		if count != set.Size() {
			t.Errorf("Backward traversal count %v != Size %v", count, set.Size())
		}
	})
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkLinkedSet_Add(b *testing.B) {
	set := NewLinkedSet[int]()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		set.Add(i)
	}
}

func BenchmarkLinkedSet_AddAll(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			items := make([]int, size)
			for i := 0; i < size; i++ {
				items[i] = i
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				set := NewLinkedSet[int]()
				set.AddAll(items...)
			}
		})
	}
}

func BenchmarkLinkedSet_Contains(b *testing.B) {
	set := NewLinkedSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		set.Contains(i % 10000)
	}
}

func BenchmarkLinkedSet_Remove(b *testing.B) {
	b.StopTimer()
	set := NewLinkedSet[int]()
	for i := 0; i < b.N; i++ {
		set.Add(i)
	}

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		set.Remove(i)
	}
}

func BenchmarkLinkedSet_Iter(b *testing.B) {
	set := NewLinkedSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range set.Iter() {
			// Iterate through all elements
		}
	}
}

func BenchmarkLinkedSet_Clone(b *testing.B) {
	set := NewLinkedSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = set.Clone()
	}
}
