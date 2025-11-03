package sets

import (
	"testing"
)

func TestNewHashSet(t *testing.T) {
	t.Run("create with default capacity", func(t *testing.T) {
		set := NewHashSet[int]()
		if set == nil {
			t.Error("NewHashSet() returned nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("create with specific capacity", func(t *testing.T) {
		set := NewHashSet[string](100)
		if set == nil {
			t.Error("NewHashSet(100) returned nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("create with multiple size parameters", func(t *testing.T) {
		set := NewHashSet[int](10, 0, 50, -1, 30)
		if set == nil {
			t.Error("NewHashSet() with multiple params returned nil")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("create with zero and negative sizes", func(t *testing.T) {
		set := NewHashSet[int](0, -10, -5)
		if set == nil {
			t.Error("NewHashSet() with non-positive sizes returned nil")
		}
	})
}

func TestHashSet_Add(t *testing.T) {
	t.Run("add single element", func(t *testing.T) {
		set := NewHashSet[int]()
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
	})

	t.Run("add duplicate element", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[string]()
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

	t.Run("add zero value", func(t *testing.T) {
		set := NewHashSet[int]()
		changed := set.Add(0)
		if !changed {
			t.Error("Add(0) = false, want true")
		}
		if !set.Contains(0) {
			t.Error("Contains(0) = false, want true")
		}
	})
}

func TestHashSet_AddAll(t *testing.T) {
	t.Run("add empty list", func(t *testing.T) {
		set := NewHashSet[int]()
		changed := set.AddAll()
		if changed {
			t.Error("AddAll() with no args = true, want false")
		}
	})

	t.Run("add multiple unique elements", func(t *testing.T) {
		set := NewHashSet[int]()
		changed := set.AddAll(1, 2, 3, 4, 5)
		if !changed {
			t.Error("AddAll(1,2,3,4,5) = false, want true")
		}
		if set.Size() != 5 {
			t.Errorf("Size() = %v, want 5", set.Size())
		}
	})

	t.Run("add with duplicates in arguments", func(t *testing.T) {
		set := NewHashSet[int]()
		changed := set.AddAll(1, 2, 2, 3, 3, 3)
		if !changed {
			t.Error("AddAll() = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})

	t.Run("add to non-empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.AddAll(3, 4, 5)
		if !changed {
			t.Error("AddAll(3,4,5) = false, want true")
		}
		if set.Size() != 5 {
			t.Errorf("Size() = %v, want 5", set.Size())
		}
	})

	t.Run("add all existing elements", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.AddAll(1, 2, 3)
		if changed {
			t.Error("AddAll() with all existing = true, want false")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})
}

func TestHashSet_Remove(t *testing.T) {
	t.Run("remove existing element", func(t *testing.T) {
		set := NewHashSet[int]()
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
	})

	t.Run("remove non-existing element", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		changed := set.Remove(1)
		if changed {
			t.Error("Remove(1) from empty = true, want false")
		}
	})

	t.Run("remove zero value", func(t *testing.T) {
		set := NewHashSet[int]()
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

func TestHashSet_RemoveAll(t *testing.T) {
	t.Run("remove empty list", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RemoveAll(2, 3, 4)
		if !changed {
			t.Error("RemoveAll(2,3,4) = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
		if !set.Contains(1) || !set.Contains(5) {
			t.Error("Wrong elements remaining after RemoveAll")
		}
	})

	t.Run("remove non-existing elements", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.RemoveAll(2, 4, 5)
		if !changed {
			t.Error("RemoveAll() mixed = false, want true")
		}
		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
	})
}

func TestHashSet_Contains(t *testing.T) {
	t.Run("contains existing element", func(t *testing.T) {
		set := NewHashSet[int]()
		set.Add(42)
		if !set.Contains(42) {
			t.Error("Contains(42) = false, want true")
		}
	})

	t.Run("contains non-existing element", func(t *testing.T) {
		set := NewHashSet[int]()
		set.Add(42)
		if set.Contains(99) {
			t.Error("Contains(99) = true, want false")
		}
	})

	t.Run("contains in empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		if set.Contains(1) {
			t.Error("Contains(1) in empty set = true, want false")
		}
	})
}

func TestHashSet_ContainsAll(t *testing.T) {
	t.Run("contains all with empty arguments", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		if !set.ContainsAll() {
			t.Error("ContainsAll() with no args = false, want true")
		}
	})

	t.Run("contains all existing elements", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		if !set.ContainsAll(2, 3, 4) {
			t.Error("ContainsAll(2,3,4) = false, want true")
		}
	})

	t.Run("contains all with one missing", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		if set.ContainsAll(1, 2, 4) {
			t.Error("ContainsAll(1,2,4) = true, want false")
		}
	})

	t.Run("contains all in empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		if set.ContainsAll(1, 2) {
			t.Error("ContainsAll(1,2) in empty = true, want false")
		}
	})

	t.Run("contains all with duplicates in arguments", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		if !set.ContainsAll(1, 1, 2, 2, 3) {
			t.Error("ContainsAll() with duplicates = false, want true")
		}
	})
}

func TestHashSet_ContainsAny(t *testing.T) {
	t.Run("contains any with empty arguments", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		if set.ContainsAny() {
			t.Error("ContainsAny() with no args = true, want false")
		}
	})

	t.Run("contains any existing element", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		if !set.ContainsAny(3, 4, 5) {
			t.Error("ContainsAny(3,4,5) = false, want true")
		}
	})

	t.Run("contains any with no matches", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		if set.ContainsAny(4, 5, 6) {
			t.Error("ContainsAny(4,5,6) = true, want false")
		}
	})

	t.Run("contains any in empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		if set.ContainsAny(1, 2, 3) {
			t.Error("ContainsAny(1,2,3) in empty = true, want false")
		}
	})
}

func TestHashSet_Retain(t *testing.T) {
	t.Run("retain existing element from multiple", func(t *testing.T) {
		set := NewHashSet[int]()
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
	})

	t.Run("retain only element", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		changed := set.Retain(99)
		if !changed {
			t.Error("Retain(99) non-existing = false, want true")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("retain from empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		changed := set.Retain(1)
		if changed {
			t.Error("Retain(1) from empty = true, want false")
		}
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})
}

func TestHashSet_RetainAll(t *testing.T) {
	t.Run("retain all with empty arguments", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		changed := set.RetainAll()
		if changed {
			t.Error("RetainAll() empty args on empty = true, want false")
		}
	})

	t.Run("retain subset", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RetainAll(2, 3, 4)
		if !changed {
			t.Error("RetainAll(2,3,4) = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
		for _, v := range []int{2, 3, 4} {
			if !set.Contains(v) {
				t.Errorf("Set should contain %v", v)
			}
		}
	})

	t.Run("retain with no intersection", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		changed := set.RetainAll(2, 2, 3, 3, 4, 4)
		if !changed {
			t.Error("RetainAll() with duplicates = false, want true")
		}
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
	})
}

func TestHashSet_Size(t *testing.T) {
	t.Run("size of empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("size after additions", func(t *testing.T) {
		set := NewHashSet[int]()
		for i := 1; i <= 10; i++ {
			set.Add(i)
			if set.Size() != i {
				t.Errorf("Size() = %v, want %v", set.Size(), i)
			}
		}
	})

	t.Run("size after removals", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		set.Remove(3)
		if set.Size() != 4 {
			t.Errorf("Size() = %v, want 4", set.Size())
		}
	})
}

func TestHashSet_IsEmpty(t *testing.T) {
	t.Run("new set is empty", func(t *testing.T) {
		set := NewHashSet[int]()
		if !set.IsEmpty() {
			t.Error("IsEmpty() = false, want true")
		}
	})

	t.Run("non-empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		set.Add(1)
		if set.IsEmpty() {
			t.Error("IsEmpty() = true, want false")
		}
	})

	t.Run("empty after clear", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()
		if !set.IsEmpty() {
			t.Error("IsEmpty() after Clear = false, want true")
		}
	})
}

func TestHashSet_Clear(t *testing.T) {
	t.Run("clear non-empty set", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
		set.Clear()
		if set.Size() != 0 {
			t.Errorf("Size() = %v, want 0", set.Size())
		}
	})

	t.Run("clear and reuse", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		set.Clear()
		set.AddAll(4, 5, 6)
		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}
		if !set.Contains(4) || !set.Contains(5) || !set.Contains(6) {
			t.Error("Set should contain 4, 5, 6 after clear and add")
		}
	})
}

func TestHashSet_Iter(t *testing.T) {
	t.Run("iterate over empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		count := 0
		for range set.Iter() {
			count++
		}
		if count != 0 {
			t.Errorf("Iterated %v times, want 0", count)
		}
	})

	t.Run("iterate over non-empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		expected := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}
		set.AddAll(1, 2, 3, 4, 5)

		count := 0
		seen := make(map[int]bool)
		for elem := range set.Iter() {
			count++
			seen[elem] = true
			if !expected[elem] {
				t.Errorf("Unexpected element %v in iteration", elem)
			}
		}

		if count != 5 {
			t.Errorf("Iterated %v times, want 5", count)
		}

		for k := range expected {
			if !seen[k] {
				t.Errorf("Element %v not seen in iteration", k)
			}
		}
	})

	t.Run("iterate with break", func(t *testing.T) {
		set := NewHashSet[int]()
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
}

func TestHashSet_ToSlice(t *testing.T) {
	t.Run("to slice from empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		slice := set.ToSlice()
		if len(slice) != 0 {
			t.Errorf("len(slice) = %v, want 0", len(slice))
		}
	})

	t.Run("to slice from non-empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3, 4, 5)
		slice := set.ToSlice()

		if len(slice) != 5 {
			t.Errorf("len(slice) = %v, want 5", len(slice))
		}

		sliceMap := make(map[int]bool)
		for _, v := range slice {
			sliceMap[v] = true
		}

		for i := 1; i <= 5; i++ {
			if !sliceMap[i] {
				t.Errorf("Slice missing element %v", i)
			}
		}
	})

	t.Run("to slice independence", func(t *testing.T) {
		set := NewHashSet[int]()
		set.AddAll(1, 2, 3)
		slice := set.ToSlice()

		slice[0] = 999
		if set.Contains(999) {
			t.Error("Modifying slice affected set")
		}
	})
}

func TestHashSet_Clone(t *testing.T) {
	t.Run("clone empty set", func(t *testing.T) {
		set := NewHashSet[int]()
		clone := set.Clone()
		if clone.Size() != 0 {
			t.Errorf("Clone size = %v, want 0", clone.Size())
		}
	})

	t.Run("clone non-empty set", func(t *testing.T) {
		set := NewHashSet[int]()
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
		set := NewHashSet[int]()
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
}

func TestHashSet_StringType(t *testing.T) {
	t.Run("string set operations", func(t *testing.T) {
		set := NewHashSet[string]()
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
	})
}

func TestHashSet_StructType(t *testing.T) {
	type Person struct {
		ID   int
		Name string
	}

	t.Run("struct set operations", func(t *testing.T) {
		set := NewHashSet[Person]()
		p1 := Person{ID: 1, Name: "Alice"}
		p2 := Person{ID: 2, Name: "Bob"}

		set.Add(p1)
		set.Add(p2)

		if !set.Contains(p1) {
			t.Error("Contains(p1) = false, want true")
		}

		if set.Size() != 2 {
			t.Errorf("Size() = %v, want 2", set.Size())
		}
	})
}

func TestHashSet_LargeDataset(t *testing.T) {
	t.Run("large dataset operations", func(t *testing.T) {
		set := NewHashSet[int](10000)
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
}
