package sets

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Constructor Tests
// ============================================================================

func TestNewSyncSet(t *testing.T) {
	t.Run("create with default HashSet", func(t *testing.T) {
		set := NewSyncSet[int]()
		if set == nil {
			t.Error("NewSyncSet() returned nil")
		}
		if set.inner == nil {
			t.Error("inner set should not be nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("create with provided HashSet", func(t *testing.T) {
		inner := NewHashSet[int]()
		inner.AddAll(1, 2, 3)

		set := NewSyncSet(inner)
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("create with provided LinkedSet", func(t *testing.T) {
		inner := NewLinkedSet[string]()
		inner.AddAll("a", "b", "c")

		set := NewSyncSet(inner)
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}

		// Verify order is preserved (LinkedSet characteristic)
		slice := set.ToSlice()
		expected := []string{"a", "b", "c"}
		for i, v := range expected {
			if slice[i] != v {
				t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
			}
		}
	})

	t.Run("prevent double wrapping", func(t *testing.T) {
		set1 := NewSyncSet[int]()
		set2 := NewSyncSet[int](set1)

		if set1 != set2 {
			t.Error("NewSyncSet() should return same instance when wrapping SyncSet")
		}
	})

	t.Run("clone isolation from original", func(t *testing.T) {
		inner := NewHashSet[int]()
		inner.Add(1)

		set := NewSyncSet(inner)
		inner.Add(2) // Modify original after wrapping

		if set.Contains(2) {
			t.Error("SyncSet should be isolated from original set modifications")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
	})

	t.Run("skip nil sets in variadic args", func(t *testing.T) {
		inner := NewHashSet[int]()
		inner.Add(42)

		set := NewSyncSet[int](nil, nil, inner, nil)
		if !set.Contains(42) {
			t.Error("Should use the first non-nil set")
		}
	})

	t.Run("all nil args creates default", func(t *testing.T) {
		set := NewSyncSet[int](nil, nil, nil)
		if set == nil {
			t.Error("Should create default HashSet")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})
}

// ============================================================================
// Basic Operations Tests
// ============================================================================

func TestSyncSet_Add(t *testing.T) {
	t.Run("add single element", func(t *testing.T) {
		set := NewSyncSet[int]()
		changed := set.Add(1)

		if !changed {
			t.Error("Add(1) = false, want true")
		}
		if !set.Contains(1) {
			t.Error("Contains(1) = false after Add")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
	})

	t.Run("add duplicate element", func(t *testing.T) {
		set := NewSyncSet[int]()
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
		set := NewSyncSet[string]()
		elements := []string{"apple", "banana", "cherry"}

		for _, elem := range elements {
			if !set.Add(elem) {
				t.Errorf("Add(%v) = false, want true", elem)
			}
		}

		if set.Size() != len(elements) {
			t.Errorf("Size() = %v, want %v", set.Size(), len(elements))
		}
	})
}

func TestSyncSet_AddAll(t *testing.T) {
	t.Run("add empty list", func(t *testing.T) {
		set := NewSyncSet[int]()
		changed := set.AddAll()

		if changed {
			t.Error("AddAll() with no args = true, want false")
		}
	})

	t.Run("add multiple elements", func(t *testing.T) {
		set := NewSyncSet[int]()
		changed := set.AddAll(1, 2, 3, 4, 5)

		if !changed {
			t.Error("AddAll(1,2,3,4,5) = false, want true")
		}
		if set.Size() != 5 {
			t.Errorf("Size() = %v, want 5", set.Size())
		}
	})

	t.Run("add with duplicates", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.Add(1)
		changed := set.AddAll(1, 2, 3)

		if !changed {
			t.Error("AddAll() should return true if any new element added")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("add all existing elements", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.AddAll(1, 2, 3)

		if changed {
			t.Error("AddAll() with all existing = true, want false")
		}
	})
}

func TestSyncSet_Remove(t *testing.T) {
	t.Run("remove existing element", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.Add(1)
		changed := set.Remove(1)

		if !changed {
			t.Error("Remove(1) = false, want true")
		}
		if set.Contains(1) {
			t.Error("Contains(1) after Remove = true, want false")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("remove non-existing element", func(t *testing.T) {
		set := NewSyncSet[int]()
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
		set := NewSyncSet[int]()
		changed := set.Remove(1)

		if changed {
			t.Error("Remove(1) from empty = true, want false")
		}
	})
}

func TestSyncSet_RemoveAll(t *testing.T) {
	t.Run("remove empty list", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll()

		if changed {
			t.Error("RemoveAll() with no args = true, want false")
		}
	})

	t.Run("remove multiple elements", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RemoveAll(2, 3, 4)

		if !changed {
			t.Error("RemoveAll(2,3,4) = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
	})

	t.Run("remove non-existing elements", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll(4, 5, 6)

		if changed {
			t.Error("RemoveAll() non-existing = true, want false")
		}
	})
}

func TestSyncSet_Contains(t *testing.T) {
	t.Run("contains existing element", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.Add(42)

		if !set.Contains(42) {
			t.Error("Contains(42) = false, want true")
		}
	})

	t.Run("contains non-existing element", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.Add(42)

		if set.Contains(99) {
			t.Error("Contains(99) = true, want false")
		}
	})

	t.Run("contains in empty set", func(t *testing.T) {
		set := NewSyncSet[int]()

		if set.Contains(1) {
			t.Error("Contains(1) in empty = true, want false")
		}
	})
}

func TestSyncSet_ContainsAll(t *testing.T) {
	t.Run("contains all with empty args", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)

		if !set.ContainsAll() {
			t.Error("ContainsAll() with no args = false, want true")
		}
	})

	t.Run("contains all existing", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		if !set.ContainsAll(2, 3, 4) {
			t.Error("ContainsAll(2,3,4) = false, want true")
		}
	})

	t.Run("contains all with one missing", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)

		if set.ContainsAll(1, 2, 4) {
			t.Error("ContainsAll(1,2,4) = true, want false")
		}
	})
}

func TestSyncSet_ContainsAny(t *testing.T) {
	t.Run("contains any with empty args", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)

		if set.ContainsAny() {
			t.Error("ContainsAny() with no args = true, want false")
		}
	})

	t.Run("contains any existing", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)

		if !set.ContainsAny(3, 4, 5) {
			t.Error("ContainsAny(3,4,5) = false, want true")
		}
	})

	t.Run("contains any with no matches", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)

		if set.ContainsAny(4, 5, 6) {
			t.Error("ContainsAny(4,5,6) = true, want false")
		}
	})
}

func TestSyncSet_Retain(t *testing.T) {
	t.Run("retain existing element", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.Retain(3)

		if !changed {
			t.Error("Retain(3) = false, want true")
		}
		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}
		if !set.Contains(3) {
			t.Error("Should contain only 3")
		}
	})

	t.Run("retain non-existing element", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.Retain(99)

		if !changed {
			t.Error("Retain(99) = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("retain from empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		changed := set.Retain(1)

		if changed {
			t.Error("Retain(1) from empty = true, want false")
		}
	})
}

func TestSyncSet_RetainAll(t *testing.T) {
	t.Run("retain all with empty args", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RetainAll()

		if !changed {
			t.Error("RetainAll() empty args from non-empty = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("retain subset", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RetainAll(2, 3, 4)

		if !changed {
			t.Error("RetainAll(2,3,4) = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("retain all elements", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RetainAll(1, 2, 3, 4, 5)

		if changed {
			t.Error("RetainAll() with all elements = true, want false")
		}
	})
}

func TestSyncSet_Size(t *testing.T) {
	t.Run("size of empty set", func(t *testing.T) {
		set := NewSyncSet[int]()

		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("size after additions", func(t *testing.T) {
		set := NewSyncSet[int]()

		for i := 1; i <= 10; i++ {
			set.Add(i)
			if set.Size() != i {
				t.Errorf("Size() = %v, want %v", set.Size(), i)
			}
		}
	})

	t.Run("size after removals", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		set.Remove(3)
		if set.Size() != 4 {
			t.Errorf("Size() = %v, want 4", set.Size())
		}
	})
}

func TestSyncSet_IsEmpty(t *testing.T) {
	t.Run("new set is empty", func(t *testing.T) {
		set := NewSyncSet[int]()

		if !set.IsEmpty() {
			t.Error("IsEmpty() = false, want true")
		}
	})

	t.Run("non-empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.Add(1)

		if set.IsEmpty() {
			t.Error("IsEmpty() = true, want false")
		}
	})

	t.Run("empty after clear", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()

		if !set.IsEmpty() {
			t.Error("IsEmpty() after Clear = false, want true")
		}
	})
}

func TestSyncSet_Clear(t *testing.T) {
	t.Run("clear non-empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Clear()

		if set.Size() != 0 {
			t.Errorf("Size() after Clear = %v, want 0", set.Size())
		}
		if !set.IsEmpty() {
			t.Error("IsEmpty() after Clear = false, want true")
		}
	})

	t.Run("clear empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.Clear()

		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("clear and reuse", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()
		set.AddAll(4, 5, 6)

		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})
}

func TestSyncSet_Iter(t *testing.T) {
	t.Run("iterate over empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		count := 0

		for range set.Iter() {
			count++
		}

		if count != 0 {
			t.Errorf("Iterated %v times, want 0", count)
		}
	})

	t.Run("iterate over non-empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)

		found := make(map[int]bool)
		for elem := range set.Iter() {
			found[elem] = true
		}

		if len(found) != 5 {
			t.Errorf("Found %v unique elements, want 5", len(found))
		}

		for i := 1; i <= 5; i++ {
			if !found[i] {
				t.Errorf("Element %v not found in iteration", i)
			}
		}
	})

	t.Run("iterate with break", func(t *testing.T) {
		set := NewSyncSet[int]()
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

	t.Run("iteration uses snapshot", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)

		count := 0
		for elem := range set.Iter() {
			count++
			// Modify set during iteration should not affect snapshot
			if elem == 1 {
				set.Add(4)
			}
		}

		if count != 3 {
			t.Errorf("Iterated %v times, want 3 (snapshot)", count)
		}

		// But the element should be in the set
		if !set.Contains(4) {
			t.Error("Contains(4) = false, want true")
		}
	})
}

func TestSyncSet_ToSlice(t *testing.T) {
	t.Run("to slice from empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		slice := set.ToSlice()

		if len(slice) != 0 {
			t.Errorf("len(slice) = %v, want 0", len(slice))
		}
	})

	t.Run("to slice from non-empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		slice := set.ToSlice()

		if len(slice) != 5 {
			t.Errorf("len(slice) = %v, want 5", len(slice))
		}

		found := make(map[int]bool)
		for _, v := range slice {
			found[v] = true
		}

		for i := 1; i <= 5; i++ {
			if !found[i] {
				t.Errorf("Element %v not found in slice", i)
			}
		}
	})

	t.Run("to slice independence", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		slice := set.ToSlice()

		slice[0] = 999

		if set.Contains(999) {
			t.Error("Modifying slice affected set")
		}
	})
}

func TestSyncSet_Clone(t *testing.T) {
	t.Run("clone empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
		clone := set.Clone()

		if clone.Size() != 0 {
			t.Errorf("Clone size = %v, want 0", clone.Size())
		}
	})

	t.Run("clone non-empty set", func(t *testing.T) {
		set := NewSyncSet[int]()
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

	t.Run("clone independence", func(t *testing.T) {
		set := NewSyncSet[int]()
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
	})

	t.Run("clone returns SyncSet", func(t *testing.T) {
		set := NewSyncSet[int]()
		set.AddAll(1, 2, 3)
		clone := set.Clone()

		syncClone, ok := clone.(*SyncSet[int])
		if !ok {
			t.Error("Clone should return *SyncSet type")
		}

		if syncClone == set {
			t.Error("Clone should be a different instance")
		}
	})
}

// ============================================================================
// Concurrency Tests
// ============================================================================

func TestSyncSet_ConcurrentReads(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 100; i++ {
		set.Add(i)
	}

	var wg sync.WaitGroup
	const numReaders = 10
	const readsPerReader = 1000

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerReader; j++ {
				set.Contains(j % 100)
			}
		}()
	}

	wg.Wait()
}

func TestSyncSet_ConcurrentWrites(t *testing.T) {
	set := NewSyncSet[int]()
	var wg sync.WaitGroup
	const numWriters = 10
	const writesPerWriter = 100

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				set.Add(start*writesPerWriter + j)
			}
		}(i)
	}

	wg.Wait()

	expectedSize := numWriters * writesPerWriter
	if set.Size() != expectedSize {
		t.Errorf("Size() = %v, want %v", set.Size(), expectedSize)
	}
}

func TestSyncSet_ConcurrentReadWrite(t *testing.T) {
	set := NewSyncSet[int]()
	done := make(chan struct{})
	const numOperations = 1000

	// Writer goroutine
	go func() {
		for i := 0; i < numOperations; i++ {
			set.Add(i)
		}
		close(done)
	}()

	// Multiple reader goroutines
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					set.Contains(500)
					set.Size()
					set.IsEmpty()
				}
			}
		}()
	}

	wg.Wait()
}

func TestSyncSet_ConcurrentAddRemove(t *testing.T) {
	set := NewSyncSet[int]()
	var wg sync.WaitGroup
	const numOperations = 1000

	// Add goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			set.Add(i)
		}
	}()

	// Remove goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			set.Remove(i)
		}
	}()

	wg.Wait()

	// Final size can be anything from 0 to numOperations depending on timing
	finalSize := set.Size()
	if finalSize < 0 || finalSize > numOperations {
		t.Errorf("Size() = %v, want 0-%v", finalSize, numOperations)
	}
}

func TestSyncSet_ConcurrentIterationAndModification(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 100; i++ {
		set.Add(i)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Iterator goroutine
	go func() {
		defer wg.Done()
		count := 0
		for range set.Iter() {
			count++
			// Slow iteration to allow concurrent modifications
			time.Sleep(time.Microsecond)
		}
		// Should complete without deadlock
	}()

	// Modifier goroutine
	go func() {
		defer wg.Done()
		for i := 100; i < 200; i++ {
			set.Add(i)
		}
	}()

	// Should complete without hanging
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

func TestSyncSet_ConcurrentBulkOperations(t *testing.T) {
	set := NewSyncSet[int]()
	var wg sync.WaitGroup

	// AddAll goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			items := make([]int, 100)
			for j := 0; j < 100; j++ {
				items[j] = start*100 + j
			}
			set.AddAll(items...)
		}(i)
	}

	// RemoveAll goroutines
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(start int) {
			defer wg.Done()
			items := make([]int, 50)
			for j := 0; j < 50; j++ {
				items[j] = start*50 + j
			}
			set.RemoveAll(items...)
		}(i)
	}

	wg.Wait()

	// Should not crash and should have valid size
	size := set.Size()
	if size < 0 {
		t.Errorf("Size() = %v, should be non-negative", size)
	}
}

func TestSyncSet_ConcurrentRetain(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 100; i++ {
		set.Add(i)
	}

	var wg sync.WaitGroup
	const numGoroutines = 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			set.Retain(n)
		}(i)
	}

	wg.Wait()

	// Should have 0 or 1 element depending on timing
	size := set.Size()
	if size > 1 {
		t.Errorf("Size() = %v, want 0 or 1", size)
	}
}

func TestSyncSet_ConcurrentClone(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 100; i++ {
		set.Add(i)
	}

	var wg sync.WaitGroup
	clones := make([]Set[int], 10)

	// Clone concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clones[idx] = set.Clone()
		}(i)
	}

	// Modify original concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 100; i < 200; i++ {
			set.Add(i)
		}
	}()

	wg.Wait()

	// All clones should have valid sizes
	for i, clone := range clones {
		size := clone.Size()
		if size < 100 || size > 200 {
			t.Errorf("Clone %v size = %v, want 100-200", i, size)
		}
	}
}

func TestSyncSet_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	set := NewSyncSet[int]()
	var wg sync.WaitGroup
	const numGoroutines = 50
	const operationsPerGoroutine = 1000

	// Mixed operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				val := id*operationsPerGoroutine + j
				switch j % 5 {
				case 0:
					set.Add(val)
				case 1:
					set.Remove(val)
				case 2:
					set.Contains(val)
				case 3:
					set.Size()
				case 4:
					for range set.Iter() {
						break // Quick iteration
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Should complete without panic or deadlock
	finalSize := set.Size()
	if finalSize < 0 {
		t.Errorf("Size() = %v, should be non-negative", finalSize)
	}
}

// ============================================================================
// Race Condition Tests (run with -race flag)
// ============================================================================

func TestSyncSet_RaceDetection(t *testing.T) {
	// This test is designed to catch race conditions with -race flag
	set := NewSyncSet[int]()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)

		go func(n int) {
			defer wg.Done()
			set.Add(n)
		}(i)

		go func(n int) {
			defer wg.Done()
			set.Contains(n)
		}(i)

		go func(n int) {
			defer wg.Done()
			set.Remove(n)
		}(i)
	}

	wg.Wait()
}

func TestSyncSet_NoDataRace(t *testing.T) {
	set := NewSyncSet[int]()
	done := make(chan bool)

	// Writer
	go func() {
		for i := 0; i < 1000; i++ {
			set.Add(i)
		}
		done <- true
	}()

	// Reader
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				set.Size()
				set.IsEmpty()
				set.Contains(500)
			}
		}
	}()

	<-done
	time.Sleep(10 * time.Millisecond) // Let reader finish
}

// ============================================================================
// Deadlock Tests
// ============================================================================

func TestSyncSet_NoDeadlockOnIterationModification(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 10; i++ {
		set.Add(i)
	}

	done := make(chan bool)
	go func() {
		// This should NOT deadlock because Iter uses snapshot
		for elem := range set.Iter() {
			set.Add(elem + 100)
			set.Remove(elem)
		}
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected in iteration with modification")
	}
}

func TestSyncSet_NoDeadlockOnNestedOperations(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 10; i++ {
		set.Add(i)
	}

	done := make(chan bool)
	go func() {
		slice := set.ToSlice()
		for _, elem := range slice {
			if set.Contains(elem) {
				set.Remove(elem)
			}
		}
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected in nested operations")
	}
}

// ============================================================================
// Performance Tests
// ============================================================================

func TestSyncSet_PerformanceComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Compare SyncSet vs regular HashSet with manual locking
	t.Run("SyncSet performance", func(t *testing.T) {
		set := NewSyncSet[int]()
		start := time.Now()

		for i := 0; i < 10000; i++ {
			set.Add(i)
			set.Contains(i / 2)
		}

		duration := time.Since(start)
		t.Logf("SyncSet: %v", duration)
	})

	t.Run("Manual mutex performance", func(t *testing.T) {
		set := NewHashSet[int]()
		var mu sync.RWMutex
		start := time.Now()

		for i := 0; i < 10000; i++ {
			mu.Lock()
			set.Add(i)
			mu.Unlock()

			mu.RLock()
			set.Contains(i / 2)
			mu.RUnlock()
		}

		duration := time.Since(start)
		t.Logf("Manual mutex: %v", duration)
	})
}

// ============================================================================
// Edge Cases and Complex Scenarios
// ============================================================================

func TestSyncSet_ConcurrentClear(t *testing.T) {
	set := NewSyncSet[int]()
	for i := 0; i < 100; i++ {
		set.Add(i)
	}

	var wg sync.WaitGroup
	const numClearers = 10

	for i := 0; i < numClearers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			set.Clear()
		}()
	}

	wg.Wait()

	if !set.IsEmpty() {
		t.Error("Set should be empty after concurrent clears")
	}
}

func TestSyncSet_HighContentionScenario(t *testing.T) {
	set := NewSyncSet[int]()
	var wg sync.WaitGroup
	var ops atomic.Int64

	// Very high contention on a small set of values
	const numGoroutines = 100
	const numOperations = 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				val := j % 10 // Only 10 different values
				set.Add(val)
				set.Contains(val)
				set.Remove(val)
				ops.Add(3)
			}
		}()
	}

	wg.Wait()

	totalOps := ops.Load()
	expectedOps := int64(numGoroutines * numOperations * 3)
	if totalOps != expectedOps {
		t.Errorf("Operations = %v, want %v", totalOps, expectedOps)
	}
}

func TestSyncSet_LinkedSetOrdering(t *testing.T) {
	inner := NewLinkedSet[int]()
	inner.AddAll(3, 1, 4, 1, 5, 9, 2, 6)

	set := NewSyncSet(inner)

	// Verify order is preserved through synchronization
	slice := set.ToSlice()
	expected := []int{3, 1, 4, 5, 9, 2, 6}

	if len(slice) != len(expected) {
		t.Errorf("len(slice) = %v, want %v", len(slice), len(expected))
	}

	for i, v := range expected {
		if slice[i] != v {
			t.Errorf("slice[%v] = %v, want %v", i, slice[i], v)
		}
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkSyncSet_Add(b *testing.B) {
	set := NewSyncSet[int]()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		set.Add(i)
	}
}

func BenchmarkSyncSet_Contains(b *testing.B) {
	set := NewSyncSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		set.Contains(i % 10000)
	}
}

func BenchmarkSyncSet_ConcurrentReads(b *testing.B) {
	set := NewSyncSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			set.Contains(5000)
		}
	})
}

func BenchmarkSyncSet_ConcurrentWrites(b *testing.B) {
	set := NewSyncSet[int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			set.Add(i)
			i++
		}
	})
}

func BenchmarkSyncSet_ConcurrentMixed(b *testing.B) {
	set := NewSyncSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				set.Add(i)
			} else {
				set.Contains(i)
			}
			i++
		}
	})
}

func BenchmarkSyncSet_Iter(b *testing.B) {
	set := NewSyncSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range set.Iter() {
			// Full iteration
		}
	}
}

func BenchmarkSyncSet_Clone(b *testing.B) {
	set := NewSyncSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = set.Clone()
	}
}

func BenchmarkSyncSet_ToSlice(b *testing.B) {
	set := NewSyncSet[int]()
	for i := 0; i < 10000; i++ {
		set.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = set.ToSlice()
	}
}

// Comparison benchmarks
func BenchmarkComparison_SyncSetVsManualMutex(b *testing.B) {
	b.Run("SyncSet", func(b *testing.B) {
		set := NewSyncSet[int]()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				set.Add(i)
				set.Contains(i)
				i++
			}
		})
	})

	b.Run("ManualMutex", func(b *testing.B) {
		set := NewHashSet[int]()
		var mu sync.RWMutex
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				mu.Lock()
				set.Add(i)
				mu.Unlock()

				mu.RLock()
				set.Contains(i)
				mu.RUnlock()

				i++
			}
		})
	})
}
