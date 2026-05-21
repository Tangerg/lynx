package sets

import (
	"testing"
)

// ============================================================================
// Union Tests
// ============================================================================

func TestUnion(t *testing.T) {
	t.Run("union of two non-empty sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(3, 4, 5)
		result := Union(s1, s2)

		if result.Size() != 5 {
			t.Errorf("Size() = %v, want 5", result.Size())
		}

		expected := []int{1, 2, 3, 4, 5}
		for _, v := range expected {
			if !result.Contains(v) {
				t.Errorf("Result should contain %v", v)
			}
		}
	})

	t.Run("union with empty set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of[int]()
		result := Union(s1, s2)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}

		for i := 1; i <= 3; i++ {
			if !result.Contains(i) {
				t.Errorf("Result should contain %v", i)
			}
		}
	})

	t.Run("union of two empty sets", func(t *testing.T) {
		s1 := Of[int]()
		s2 := Of[int]()
		result := Union(s1, s2)

		if !result.IsEmpty() {
			t.Error("Union of empty sets should be empty")
		}
	})

	t.Run("union with identical sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)
		result := Union(s1, s2)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}
	})

	t.Run("union independence from original sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(3, 4, 5)
		result := Union(s1, s2)

		s1.Add(99)
		s2.Add(100)

		if result.Contains(99) || result.Contains(100) {
			t.Error("Result should be independent of original sets")
		}
	})
}

func TestUnionAll(t *testing.T) {
	t.Run("union of no sets", func(t *testing.T) {
		result := UnionAll[int]()

		if !result.IsEmpty() {
			t.Error("UnionAll with no sets should return empty set")
		}
	})

	t.Run("union of single set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		result := UnionAll(s1)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}

		// Should be a clone, not the same instance
		s1.Add(4)
		if result.Contains(4) {
			t.Error("Result should be independent of original set")
		}
	})

	t.Run("union of multiple sets", func(t *testing.T) {
		s1 := Of(1, 2)
		s2 := Of(2, 3)
		s3 := Of(3, 4)
		s4 := Of(4, 5)
		result := UnionAll(s1, s2, s3, s4)

		if result.Size() != 5 {
			t.Errorf("Size() = %v, want 5", result.Size())
		}

		for i := 1; i <= 5; i++ {
			if !result.Contains(i) {
				t.Errorf("Result should contain %v", i)
			}
		}
	})

	t.Run("union with overlapping sets", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4, 5)
		s2 := Of(3, 4, 5, 6, 7)
		s3 := Of(5, 6, 7, 8, 9)
		result := UnionAll(s1, s2, s3)

		if result.Size() != 9 {
			t.Errorf("Size() = %v, want 9", result.Size())
		}
	})

	t.Run("union with empty sets mixed in", func(t *testing.T) {
		s1 := Of(1, 2)
		s2 := Of[int]()
		s3 := Of(3, 4)
		result := UnionAll(s1, s2, s3)

		if result.Size() != 4 {
			t.Errorf("Size() = %v, want 4", result.Size())
		}
	})
}

// ============================================================================
// Intersection Tests
// ============================================================================

func TestIntersection(t *testing.T) {
	t.Run("intersection of two sets with common elements", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4)
		s2 := Of(3, 4, 5, 6)
		result := Intersection(s1, s2)

		if result.Size() != 2 {
			t.Errorf("Size() = %v, want 2", result.Size())
		}

		if !result.Contains(3) || !result.Contains(4) {
			t.Error("Result should contain 3 and 4")
		}
	})

	t.Run("intersection with no common elements", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(4, 5, 6)
		result := Intersection(s1, s2)

		if !result.IsEmpty() {
			t.Error("Intersection with no common elements should be empty")
		}
	})

	t.Run("intersection with empty set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of[int]()
		result := Intersection(s1, s2)

		if !result.IsEmpty() {
			t.Error("Intersection with empty set should be empty")
		}
	})

	t.Run("intersection of identical sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)
		result := Intersection(s1, s2)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}
	})

	t.Run("intersection optimizes with smaller set", func(t *testing.T) {
		small := Of(1, 2)
		large := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
		result := Intersection(small, large)

		if result.Size() != 2 {
			t.Errorf("Size() = %v, want 2", result.Size())
		}

		// Verify both orders work
		result2 := Intersection(large, small)
		if result2.Size() != 2 {
			t.Errorf("Size() = %v, want 2", result2.Size())
		}
	})
}

func TestIntersectionAll(t *testing.T) {
	t.Run("intersection of no sets", func(t *testing.T) {
		result := IntersectionAll[int]()

		if !result.IsEmpty() {
			t.Error("IntersectionAll with no sets should return empty set")
		}
	})

	t.Run("intersection of single set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		result := IntersectionAll(s1)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}

		// Should be a clone
		s1.Add(4)
		if result.Contains(4) {
			t.Error("Result should be independent of original set")
		}
	})

	t.Run("intersection of multiple sets", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4, 5)
		s2 := Of(2, 3, 4, 5, 6)
		s3 := Of(3, 4, 5, 6, 7)
		result := IntersectionAll(s1, s2, s3)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}

		expected := []int{3, 4, 5}
		for _, v := range expected {
			if !result.Contains(v) {
				t.Errorf("Result should contain %v", v)
			}
		}
	})

	t.Run("intersection with empty result", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(2, 3, 4)
		s3 := Of(5, 6, 7)
		result := IntersectionAll(s1, s2, s3)

		if !result.IsEmpty() {
			t.Error("Intersection with no common elements should be empty")
		}
	})

	t.Run("intersection early termination", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(4, 5, 6)
		s3 := Of(7, 8, 9) // This shouldn't be evaluated
		result := IntersectionAll(s1, s2, s3)

		if !result.IsEmpty() {
			t.Error("Should terminate early when intersection becomes empty")
		}
	})
}

// ============================================================================
// Difference Tests
// ============================================================================

func TestDifference(t *testing.T) {
	t.Run("difference of two sets", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4)
		s2 := Of(3, 4, 5, 6)
		result := Difference(s1, s2)

		if result.Size() != 2 {
			t.Errorf("Size() = %v, want 2", result.Size())
		}

		if !result.Contains(1) || !result.Contains(2) {
			t.Error("Result should contain 1 and 2")
		}

		if result.Contains(3) || result.Contains(4) {
			t.Error("Result should not contain 3 or 4")
		}
	})

	t.Run("difference with no overlap", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(4, 5, 6)
		result := Difference(s1, s2)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}
	})

	t.Run("difference with empty set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of[int]()
		result := Difference(s1, s2)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}
	})

	t.Run("difference resulting in empty set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3, 4, 5)
		result := Difference(s1, s2)

		if !result.IsEmpty() {
			t.Error("Difference should be empty when s1 ⊆ s2")
		}
	})

	t.Run("difference is not commutative", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(2, 3, 4)

		result1 := Difference(s1, s2)
		result2 := Difference(s2, s1)

		if result1.Contains(1) != true {
			t.Error("s1 - s2 should contain 1")
		}

		if result2.Contains(4) != true {
			t.Error("s2 - s1 should contain 4")
		}

		if Equal(result1, result2) {
			t.Error("Difference should not be commutative")
		}
	})
}

func TestDifferenceAll(t *testing.T) {
	t.Run("difference of no sets", func(t *testing.T) {
		result := DifferenceAll[int]()

		if !result.IsEmpty() {
			t.Error("DifferenceAll with no sets should return empty set")
		}
	})

	t.Run("difference of single set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		result := DifferenceAll(s1)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}

		// Should be a clone
		s1.Add(4)
		if result.Contains(4) {
			t.Error("Result should be independent of original set")
		}
	})

	t.Run("sequential difference", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4, 5)
		s2 := Of(2, 3)
		s3 := Of(4)
		result := DifferenceAll(s1, s2, s3)

		if result.Size() != 2 {
			t.Errorf("Size() = %v, want 2", result.Size())
		}

		if !result.Contains(1) || !result.Contains(5) {
			t.Error("Result should contain 1 and 5")
		}
	})

	t.Run("difference early termination", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3, 4, 5)
		s3 := Of(6, 7, 8) // Shouldn't matter
		result := DifferenceAll(s1, s2, s3)

		if !result.IsEmpty() {
			t.Error("Should terminate early when result becomes empty")
		}
	})
}

func TestSymmetricDifference(t *testing.T) {
	t.Run("symmetric difference of two sets", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4)
		s2 := Of(3, 4, 5, 6)
		result := SymmetricDifference(s1, s2)

		if result.Size() != 4 {
			t.Errorf("Size() = %v, want 4", result.Size())
		}

		expected := []int{1, 2, 5, 6}
		for _, v := range expected {
			if !result.Contains(v) {
				t.Errorf("Result should contain %v", v)
			}
		}

		notExpected := []int{3, 4}
		for _, v := range notExpected {
			if result.Contains(v) {
				t.Errorf("Result should not contain %v", v)
			}
		}
	})

	t.Run("symmetric difference is commutative", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(3, 4, 5)

		result1 := SymmetricDifference(s1, s2)
		result2 := SymmetricDifference(s2, s1)

		if !Equal(result1, result2) {
			t.Error("Symmetric difference should be commutative")
		}
	})

	t.Run("symmetric difference with no overlap", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(4, 5, 6)
		result := SymmetricDifference(s1, s2)

		if result.Size() != 6 {
			t.Errorf("Size() = %v, want 6", result.Size())
		}
	})

	t.Run("symmetric difference with identical sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)
		result := SymmetricDifference(s1, s2)

		if !result.IsEmpty() {
			t.Error("Symmetric difference of identical sets should be empty")
		}
	})

	t.Run("symmetric difference with empty set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of[int]()
		result := SymmetricDifference(s1, s2)

		if result.Size() != 3 {
			t.Errorf("Size() = %v, want 3", result.Size())
		}
	})
}

// ============================================================================
// Comparison Tests
// ============================================================================

func TestEqual(t *testing.T) {
	t.Run("equal sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(3, 2, 1) // Different order

		if !Equal(s1, s2) {
			t.Error("Sets with same elements should be equal")
		}
	})

	t.Run("unequal sets different sizes", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3, 4)

		if Equal(s1, s2) {
			t.Error("Sets with different sizes should not be equal")
		}
	})

	t.Run("unequal sets same size", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 4)

		if Equal(s1, s2) {
			t.Error("Sets with different elements should not be equal")
		}
	})

	t.Run("empty sets are equal", func(t *testing.T) {
		s1 := Of[int]()
		s2 := Of[int]()

		if !Equal(s1, s2) {
			t.Error("Empty sets should be equal")
		}
	})

	t.Run("equal optimizes with smaller set", func(t *testing.T) {
		small := Of(1, 2)
		large := Of(2, 1)

		if !Equal(small, large) {
			t.Error("Should be equal")
		}
	})
}

func TestIsSubset(t *testing.T) {
	t.Run("proper subset", func(t *testing.T) {
		s1 := Of(1, 2)
		s2 := Of(1, 2, 3, 4)

		if !IsSubset(s1, s2) {
			t.Error("s1 should be a subset of s2")
		}
	})

	t.Run("not a subset", func(t *testing.T) {
		s1 := Of(1, 2, 5)
		s2 := Of(1, 2, 3, 4)

		if IsSubset(s1, s2) {
			t.Error("s1 should not be a subset of s2")
		}
	})

	t.Run("equal sets are subsets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)

		if !IsSubset(s1, s2) {
			t.Error("Equal sets should be subsets of each other")
		}
	})

	t.Run("empty set is subset of any set", func(t *testing.T) {
		s1 := Of[int]()
		s2 := Of(1, 2, 3)

		if !IsSubset(s1, s2) {
			t.Error("Empty set should be subset of any set")
		}
	})

	t.Run("larger set cannot be subset", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4, 5)
		s2 := Of(1, 2, 3)

		if IsSubset(s1, s2) {
			t.Error("Larger set cannot be subset of smaller set")
		}
	})
}

func TestIsSuperset(t *testing.T) {
	t.Run("proper superset", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4)
		s2 := Of(1, 2)

		if !IsSuperset(s1, s2) {
			t.Error("s1 should be a superset of s2")
		}
	})

	t.Run("not a superset", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 5)

		if IsSuperset(s1, s2) {
			t.Error("s1 should not be a superset of s2")
		}
	})

	t.Run("equal sets are supersets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)

		if !IsSuperset(s1, s2) {
			t.Error("Equal sets should be supersets of each other")
		}
	})
}

func TestIsProperSubset(t *testing.T) {
	t.Run("proper subset", func(t *testing.T) {
		s1 := Of(1, 2)
		s2 := Of(1, 2, 3)

		if !IsProperSubset(s1, s2) {
			t.Error("s1 should be a proper subset of s2")
		}
	})

	t.Run("equal sets are not proper subsets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)

		if IsProperSubset(s1, s2) {
			t.Error("Equal sets should not be proper subsets")
		}
	})

	t.Run("empty set is proper subset of non-empty", func(t *testing.T) {
		s1 := Of[int]()
		s2 := Of(1, 2, 3)

		if !IsProperSubset(s1, s2) {
			t.Error("Empty set should be proper subset of non-empty set")
		}
	})

	t.Run("not a proper subset", func(t *testing.T) {
		s1 := Of(1, 2, 5)
		s2 := Of(1, 2, 3, 4)

		if IsProperSubset(s1, s2) {
			t.Error("s1 should not be a proper subset")
		}
	})
}

func TestIsProperSuperset(t *testing.T) {
	t.Run("proper superset", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2)

		if !IsProperSuperset(s1, s2) {
			t.Error("s1 should be a proper superset of s2")
		}
	})

	t.Run("equal sets are not proper supersets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(1, 2, 3)

		if IsProperSuperset(s1, s2) {
			t.Error("Equal sets should not be proper supersets")
		}
	})

	t.Run("non-empty is proper superset of empty", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of[int]()

		if !IsProperSuperset(s1, s2) {
			t.Error("Non-empty set should be proper superset of empty set")
		}
	})
}

func TestIsDisjoint(t *testing.T) {
	t.Run("disjoint sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(4, 5, 6)

		if !IsDisjoint(s1, s2) {
			t.Error("Sets with no common elements should be disjoint")
		}
	})

	t.Run("non-disjoint sets", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(3, 4, 5)

		if IsDisjoint(s1, s2) {
			t.Error("Sets with common elements should not be disjoint")
		}
	})

	t.Run("empty sets are disjoint", func(t *testing.T) {
		s1 := Of[int]()
		s2 := Of[int]()

		if !IsDisjoint(s1, s2) {
			t.Error("Empty sets should be disjoint")
		}
	})

	t.Run("empty set is disjoint with non-empty", func(t *testing.T) {
		s1 := Of[int]()
		s2 := Of(1, 2, 3)

		if !IsDisjoint(s1, s2) {
			t.Error("Empty set should be disjoint with any set")
		}
	})

	t.Run("disjoint optimizes with smaller set", func(t *testing.T) {
		small := Of(100, 200)
		large := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)

		if !IsDisjoint(small, large) {
			t.Error("Should be disjoint")
		}

		// Verify both orders work
		if !IsDisjoint(large, small) {
			t.Error("Should be disjoint (reversed)")
		}
	})
}

// ============================================================================
// CartesianProduct Tests
// ============================================================================

func TestCartesianProduct(t *testing.T) {
	t.Run("basic cartesian product", func(t *testing.T) {
		s1 := Of(1, 2)
		s2 := Of("a", "b")
		result := CartesianProduct(s1, s2)

		if result.Size() != 4 {
			t.Errorf("Size() = %v, want 4", result.Size())
		}

		expected := []Pair[int, string]{
			{1, "a"}, {1, "b"}, {2, "a"}, {2, "b"},
		}

		for _, pair := range expected {
			if !result.Contains(pair) {
				t.Errorf("Result should contain %v", pair)
			}
		}
	})

	t.Run("cartesian product with empty set", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of[string]()
		result := CartesianProduct(s1, s2)

		if !result.IsEmpty() {
			t.Error("Cartesian product with empty set should be empty")
		}
	})

	t.Run("cartesian product of single element sets", func(t *testing.T) {
		s1 := Of(42)
		s2 := Of("x")
		result := CartesianProduct(s1, s2)

		if result.Size() != 1 {
			t.Errorf("Size() = %v, want 1", result.Size())
		}

		if !result.Contains(Pair[int, string]{42, "x"}) {
			t.Error("Result should contain pair (42, x)")
		}
	})

	t.Run("cartesian product different types", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(true, false)
		result := CartesianProduct(s1, s2)

		if result.Size() != 6 {
			t.Errorf("Size() = %v, want 6", result.Size())
		}
	})

	t.Run("pair string representation", func(t *testing.T) {
		pair := Pair[int, string]{First: 1, Second: "a"}
		str := pair.String()
		expected := "(1, a)"

		if str != expected {
			t.Errorf("String() = %v, want %v", str, expected)
		}
	})

	t.Run("cartesian product size verification", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4, 5)
		s2 := Of("a", "b", "c")
		result := CartesianProduct(s1, s2)

		expectedSize := s1.Size() * s2.Size()
		if result.Size() != expectedSize {
			t.Errorf("Size() = %v, want %v", result.Size(), expectedSize)
		}
	})

	t.Run("cartesian product uniqueness", func(t *testing.T) {
		s1 := Of(1, 1, 2, 2)    // Duplicates
		s2 := Of("a", "a", "b") // Duplicates
		result := CartesianProduct(s1, s2)

		// After deduplication: s1={1,2}, s2={a,b}, so 2*2=4 pairs
		if result.Size() != 4 {
			t.Errorf("Size() = %v, want 4", result.Size())
		}
	})
}

// ============================================================================
// Of Function Tests
// ============================================================================

func TestOf(t *testing.T) {
	t.Run("create set from elements", func(t *testing.T) {
		set := Of(1, 2, 3)

		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3", set.Size())
		}

		for i := 1; i <= 3; i++ {
			if !set.Contains(i) {
				t.Errorf("Set should contain %v", i)
			}
		}
	})

	t.Run("create set with duplicates", func(t *testing.T) {
		set := Of(1, 2, 3, 2, 1)

		if set.Size() != 3 {
			t.Errorf("Size() = %v, want 3 (duplicates removed)", set.Size())
		}
	})

	t.Run("create empty set", func(t *testing.T) {
		set := Of[string]()

		if !set.IsEmpty() {
			t.Error("Set should be empty")
		}
	})

	t.Run("create set with single element", func(t *testing.T) {
		set := Of(42)

		if set.Size() != 1 {
			t.Errorf("Size() = %v, want 1", set.Size())
		}

		if !set.Contains(42) {
			t.Error("Set should contain 42")
		}
	})

	t.Run("create set with different types", func(t *testing.T) {
		intSet := Of(1, 2, 3)
		stringSet := Of("a", "b", "c")
		boolSet := Of(true, false)

		if intSet.Size() != 3 || stringSet.Size() != 3 || boolSet.Size() != 2 {
			t.Error("Sets should have correct sizes")
		}
	})
}

// ============================================================================
// Complex Scenario Tests
// ============================================================================

func TestComplexSetOperations(t *testing.T) {
	t.Run("multiple operations chain", func(t *testing.T) {
		s1 := Of(1, 2, 3, 4, 5)
		s2 := Of(4, 5, 6, 7, 8)
		s3 := Of(1, 3, 5, 7, 9)

		// (s1 ∪ s2) ∩ s3
		union := Union(s1, s2)
		result := Intersection(union, s3)

		expected := []int{1, 3, 5, 7}
		if result.Size() != len(expected) {
			t.Errorf("Size() = %v, want %v", result.Size(), len(expected))
		}

		for _, v := range expected {
			if !result.Contains(v) {
				t.Errorf("Result should contain %v", v)
			}
		}
	})

	t.Run("de morgans law union", func(t *testing.T) {
		// ¬(A ∪ B) = ¬A ∩ ¬B
		universal := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
		s1 := Of(1, 2, 3)
		s2 := Of(4, 5, 6)

		// Left side: ¬(A ∪ B)
		unionSet := Union(s1, s2)
		left := Difference(universal, unionSet)

		// Right side: ¬A ∩ ¬B
		notA := Difference(universal, s1)
		notB := Difference(universal, s2)
		right := Intersection(notA, notB)

		if !Equal(left, right) {
			t.Error("De Morgan's law for union should hold")
		}
	})

	t.Run("de morgans law intersection", func(t *testing.T) {
		// ¬(A ∩ B) = ¬A ∪ ¬B
		universal := Of(1, 2, 3, 4, 5, 6, 7, 8)
		s1 := Of(1, 2, 3, 4)
		s2 := Of(3, 4, 5, 6)

		// Left side: ¬(A ∩ B)
		intersectionSet := Intersection(s1, s2)
		left := Difference(universal, intersectionSet)

		// Right side: ¬A ∪ ¬B
		notA := Difference(universal, s1)
		notB := Difference(universal, s2)
		right := Union(notA, notB)

		if !Equal(left, right) {
			t.Error("De Morgan's law for intersection should hold")
		}
	})

	t.Run("distributive law", func(t *testing.T) {
		// A ∩ (B ∪ C) = (A ∩ B) ∪ (A ∩ C)
		s1 := Of(1, 2, 3, 4)
		s2 := Of(3, 4, 5, 6)
		s3 := Of(2, 4, 6, 8)

		// Left side: A ∩ (B ∪ C)
		unionBC := Union(s2, s3)
		left := Intersection(s1, unionBC)

		// Right side: (A ∩ B) ∪ (A ∩ C)
		intersectionAB := Intersection(s1, s2)
		intersectionAC := Intersection(s1, s3)
		right := Union(intersectionAB, intersectionAC)

		if !Equal(left, right) {
			t.Error("Distributive law should hold")
		}
	})

	t.Run("symmetric difference properties", func(t *testing.T) {
		s1 := Of(1, 2, 3)
		s2 := Of(3, 4, 5)

		// A Δ B = (A - B) ∪ (B - A)
		diffAB := Difference(s1, s2)
		diffBA := Difference(s2, s1)
		expected := Union(diffAB, diffBA)

		result := SymmetricDifference(s1, s2)

		if !Equal(result, expected) {
			t.Error("Symmetric difference should equal (A-B) ∪ (B-A)")
		}
	})
}

// ============================================================================
// Edge Cases Tests
// ============================================================================

func TestEdgeCases(t *testing.T) {
	t.Run("operations on same set reference", func(t *testing.T) {
		set := Of(1, 2, 3)

		union := Union(set, set)
		if !Equal(union, set) {
			t.Error("Union of set with itself should equal the set")
		}

		intersection := Intersection(set, set)
		if !Equal(intersection, set) {
			t.Error("Intersection of set with itself should equal the set")
		}

		difference := Difference(set, set)
		if !difference.IsEmpty() {
			t.Error("Difference of set with itself should be empty")
		}

		symDiff := SymmetricDifference(set, set)
		if !symDiff.IsEmpty() {
			t.Error("Symmetric difference of set with itself should be empty")
		}
	})

	t.Run("large sets performance", func(t *testing.T) {
		// Create large sets
		large1 := NewHashSet[int](10000)
		large2 := NewHashSet[int](10000)

		for i := 0; i < 10000; i++ {
			large1.Add(i)
			large2.Add(i + 5000) // 50% overlap
		}

		// These should complete without hanging
		_ = Union(large1, large2)
		_ = Intersection(large1, large2)
		_ = Difference(large1, large2)
		_ = SymmetricDifference(large1, large2)
	})

	t.Run("subset and superset reflexivity", func(t *testing.T) {
		set := Of(1, 2, 3)

		if !IsSubset(set, set) {
			t.Error("Every set should be a subset of itself")
		}

		if !IsSuperset(set, set) {
			t.Error("Every set should be a superset of itself")
		}

		if IsProperSubset(set, set) {
			t.Error("No set should be a proper subset of itself")
		}

		if IsProperSuperset(set, set) {
			t.Error("No set should be a proper superset of itself")
		}
	})

	t.Run("disjoint with itself", func(t *testing.T) {
		emptySet := Of[int]()
		if !IsDisjoint(emptySet, emptySet) {
			t.Error("Empty set should be disjoint with itself")
		}

		nonEmpty := Of(1, 2, 3)
		if IsDisjoint(nonEmpty, nonEmpty) {
			t.Error("Non-empty set should not be disjoint with itself")
		}
	})
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkUnion(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	s2 := Of(6, 7, 8, 9, 10, 11, 12, 13, 14, 15)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Union(s1, s2)
	}
}

func BenchmarkIntersection(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	s2 := Of(6, 7, 8, 9, 10, 11, 12, 13, 14, 15)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Intersection(s1, s2)
	}
}

func BenchmarkDifference(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	s2 := Of(6, 7, 8, 9, 10, 11, 12, 13, 14, 15)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Difference(s1, s2)
	}
}

func BenchmarkSymmetricDifference(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	s2 := Of(6, 7, 8, 9, 10, 11, 12, 13, 14, 15)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SymmetricDifference(s1, s2)
	}
}

func BenchmarkEqual(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	s2 := Of(10, 9, 8, 7, 6, 5, 4, 3, 2, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Equal(s1, s2)
	}
}

func BenchmarkIsSubset(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5)
	s2 := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsSubset(s1, s2)
	}
}

func BenchmarkIsDisjoint(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5)
	s2 := Of(6, 7, 8, 9, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = IsDisjoint(s1, s2)
	}
}

func BenchmarkCartesianProduct(b *testing.B) {
	s1 := Of(1, 2, 3, 4, 5)
	s2 := Of("a", "b", "c", "d", "e")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CartesianProduct(s1, s2)
	}
}

func BenchmarkOf(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	}
}
