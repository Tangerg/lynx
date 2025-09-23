package sets

import "fmt"

// Union creates a new set containing all elements from both input sets.
// This is a convenience function that delegates to UnionAll for consistency.
//
// Time complexity: O(|s1| + |s2|)
// Space complexity: O(|s1| + |s2|)
//
// Example:
//
//	s1 := Of(1, 2, 3)
//	s2 := Of(3, 4, 5)
//	result := Union(s1, s2)  // contains {1, 2, 3, 4, 5}
func Union[T comparable](s1, s2 Set[T]) Set[T] {
	return UnionAll(s1, s2)
}

// Intersection creates a new set containing only elements present in both input sets.
// This is a convenience function that delegates to IntersectionAll for consistency.
//
// Time complexity: O(min(|s1|, |s2|))
// Space complexity: O(min(|s1|, |s2|))
//
// Example:
//
//	s1 := Of(1, 2, 3, 4)
//	s2 := Of(3, 4, 5, 6)
//	result := Intersection(s1, s2)  // contains {3, 4}
func Intersection[T comparable](s1, s2 Set[T]) Set[T] {
	return IntersectionAll(s1, s2)
}

// Difference creates a new set containing elements in s1 but not in s2.
// This is a convenience function that delegates to DifferenceAll for consistency.
//
// Time complexity: O(|s1| + |s2|)
// Space complexity: O(|s1|)
//
// Example:
//
//	s1 := Of(1, 2, 3, 4)
//	s2 := Of(3, 4, 5, 6)
//	result := Difference(s1, s2)  // contains {1, 2}
func Difference[T comparable](s1, s2 Set[T]) Set[T] {
	return DifferenceAll(s1, s2)
}

// SymmetricDifference creates a new set containing elements in either s1 or s2 but not in both.
// This performs the symmetric difference operation: (s1 - s2) ∪ (s2 - s1).
//
// Time complexity: O(|s1| + |s2|)
// Space complexity: O(|s1| + |s2|)
//
// The symmetric difference is commutative: SymmetricDifference(A, B) = SymmetricDifference(B, A)
//
// Example:
//
//	s1 := Of(1, 2, 3, 4)
//	s2 := Of(3, 4, 5, 6)
//	result := SymmetricDifference(s1, s2)  // contains {1, 2, 5, 6}
func SymmetricDifference[T comparable](s1, s2 Set[T]) Set[T] {
	// Pre-estimate capacity for worst case (no overlapping elements)
	result := NewHashSet[T](s1.Size() + s2.Size())

	// Add elements from s1 that are not in s2
	for x := range s1.Iter() {
		if !s2.Contains(x) {
			result.Add(x)
		}
	}

	// Add elements from s2 that are not in s1
	for x := range s2.Iter() {
		if !s1.Contains(x) {
			result.Add(x)
		}
	}

	return result
}

// UnionAll creates a new set containing all elements from multiple input sets.
// This is the core implementation for union operations.
//
// Time complexity: O(∑|si|) where the sum is over all input sets.
// Space complexity: O(∑|si|) for the result set in the worst case.
//
// Special cases:
//   - No sets: returns empty set
//   - One set: returns clone of that set
//   - Multiple sets: general case with estimated capacity
//
// Example:
//
//	s1 := Of(1, 2)
//	s2 := Of(2, 3)
//	s3 := Of(3, 4)
//	result := UnionAll(s1, s2, s3)  // contains {1, 2, 3, 4}
func UnionAll[T comparable](sets ...Set[T]) Set[T] {
	switch len(sets) {
	case 0:
		return NewHashSet[T]()
	case 1:
		return sets[0].Clone()
	default:
		// General case: estimate total capacity to minimize reallocations
		totalSize := 0
		for _, s := range sets {
			totalSize += s.Size()
		}

		result := NewHashSet[T](totalSize)

		// Add all elements from all sets
		for _, s := range sets {
			for x := range s.Iter() {
				result.Add(x)
			}
		}

		return result
	}
}

// IntersectionAll creates a new set containing only elements present in all input sets.
// This is the core implementation for intersection operations.
//
// Time complexity: O(∑|si|) in the worst case, but often much better due to optimizations.
// Space complexity: O(min(|si|)) for the result set.
//
// Special cases:
//   - No sets: returns empty set
//   - One set: returns clone of that set
//   - Two sets: optimized path using smaller set iteration
//   - Multiple sets: sequential intersection with early termination
//
// Example:
//
//	s1 := Of(1, 2, 3, 4)
//	s2 := Of(2, 3, 4, 5)
//	s3 := Of(3, 4, 5, 6)
//	result := IntersectionAll(s1, s2, s3)  // contains {3, 4}
func IntersectionAll[T comparable](sets ...Set[T]) Set[T] {
	switch len(sets) {
	case 0:
		return NewHashSet[T]()
	case 1:
		return sets[0].Clone()
	case 2:
		// Optimized path for two sets
		s1, s2 := sets[0], sets[1]

		// Choose the smaller set for iteration to minimize operations
		smaller, larger := s1, s2
		if s2.Size() < s1.Size() {
			smaller, larger = s2, s1
		}

		// Pre-allocate capacity based on the smaller set size (upper bound)
		result := NewHashSet[T](smaller.Size())

		// Iterate over the smaller set and check membership in the larger set
		for x := range smaller.Iter() {
			if larger.Contains(x) {
				result.Add(x)
			}
		}

		return result
	default:
		// General case: start with first set and intersect with others
		result := sets[0].Clone()

		// Intersect with each subsequent set
		for i := 1; i < len(sets); i++ {
			if result.IsEmpty() {
				break // Early termination - empty intersection
			}
			result = IntersectionAll(result, sets[i]) // Recursive call to two-set case
		}

		return result
	}
}

// DifferenceAll performs sequential difference operations: s1 - s2 - s3 - ... - sn.
// This is the core implementation for difference operations.
//
// Time complexity: O(∑|si|) in the worst case, but often better due to early termination.
// Space complexity: O(|s1|) for the result set.
//
// Special cases:
//   - No sets: returns empty set
//   - One set: returns clone of that set
//   - Two sets: optimized single-pass implementation
//   - Multiple sets: sequential difference with early termination
//
// Example:
//
//	s1 := Of(1, 2, 3, 4, 5)
//	s2 := Of(2, 3)
//	s3 := Of(4)
//	result := DifferenceAll(s1, s2, s3)  // contains {1, 5}
func DifferenceAll[T comparable](sets ...Set[T]) Set[T] {
	switch len(sets) {
	case 0:
		return NewHashSet[T]()
	case 1:
		return sets[0].Clone()
	case 2:
		// Optimized path for two sets
		s1, s2 := sets[0], sets[1]

		// Pre-allocate capacity based on s1 size (upper bound for result)
		result := NewHashSet[T](s1.Size())

		// Add elements from s1 that are not present in s2
		for x := range s1.Iter() {
			if !s2.Contains(x) {
				result.Add(x)
			}
		}

		return result
	default:
		// General case: start with first set and subtract others
		result := sets[0].Clone()

		// Subtract each subsequent set
		for i := 1; i < len(sets); i++ {
			if result.IsEmpty() {
				break // Early termination - empty result
			}
			result = DifferenceAll(result, sets[i]) // Recursive call to two-set case
		}

		return result
	}
}

// Equal checks if two sets contain exactly the same elements.
// Returns true if both sets have the same size and contain the same elements.
//
// Time complexity: O(min(|s1|, |s2|)) with early termination optimizations.
// Space complexity: O(1) - no additional space required.
//
// The function includes several optimizations:
// - Fast size comparison for early rejection
// - Empty set handling
// - Iteration over the smaller set for efficiency
//
// Example:
//
//	s1 := Of(1, 2, 3)
//	s2 := Of(3, 2, 1)  // different order, same elements
//	result := Equal(s1, s2)  // returns true
func Equal[T comparable](s1, s2 Set[T]) bool {
	// Quick size check - different sizes mean different sets
	if s1.Size() != s2.Size() {
		return false
	}

	// Both empty sets are equal
	if s1.IsEmpty() {
		return true
	}

	// Choose smaller set for iteration (sizes should be equal, but defensive programming)
	smaller, larger := s1, s2
	if s2.Size() < s1.Size() {
		smaller, larger = s2, s1
	}

	// Check if all elements in smaller set exist in larger set
	for x := range smaller.Iter() {
		if !larger.Contains(x) {
			return false
		}
	}

	return true
}

// IsSubset checks if s1 is a subset of s2 (s1 ⊆ s2).
// Returns true if every element in s1 is also in s2.
// An empty set is a subset of any set, and every set is a subset of itself.
//
// Time complexity: O(|s1|) - iterates through s1 and performs lookups in s2.
// Space complexity: O(1) - no additional space required.
//
// Example:
//
//	s1 := Of(1, 2)
//	s2 := Of(1, 2, 3, 4)
//	result := IsSubset(s1, s2)  // returns true
func IsSubset[T comparable](s1, s2 Set[T]) bool {
	// Quick optimization: if s1 is larger than s2, it cannot be a subset
	if s1.Size() > s2.Size() {
		return false
	}

	// Check if every element in s1 exists in s2
	for x := range s1.Iter() {
		if !s2.Contains(x) {
			return false
		}
	}

	return true
}

// IsSuperset checks if s1 is a superset of s2 (s1 ⊇ s2).
// Returns true if s1 contains every element that is in s2.
// This is equivalent to IsSubset(s2, s1).
//
// Time complexity: O(|s2|) - delegates to IsSubset with swapped arguments.
// Space complexity: O(1) - no additional space required.
//
// Example:
//
//	s1 := Of(1, 2, 3, 4)
//	s2 := Of(1, 2)
//	result := IsSuperset(s1, s2)  // returns true
func IsSuperset[T comparable](s1, s2 Set[T]) bool {
	return IsSubset(s2, s1)
}

// IsProperSubset checks if s1 is a proper subset of s2 (s1 ⊂ s2).
// Returns true if s1 is a subset of s2 AND s1 ≠ s2 (i.e., s2 has more elements).
//
// Time complexity: O(|s1|) - includes size comparison and subset check.
// Space complexity: O(1) - no additional space required.
//
// A proper subset must be strictly smaller than the superset.
//
// Example:
//
//	s1 := Of(1, 2)
//	s2 := Of(1, 2, 3)
//	result := IsProperSubset(s1, s2)  // returns true
func IsProperSubset[T comparable](s1, s2 Set[T]) bool {
	return s1.Size() < s2.Size() && IsSubset(s1, s2)
}

// IsProperSuperset checks if s1 is a proper superset of s2 (s1 ⊃ s2).
// Returns true if s1 is a superset of s2 AND s1 ≠ s2 (i.e., s1 has more elements).
// This is equivalent to IsProperSubset(s2, s1).
//
// Time complexity: O(|s2|) - delegates to IsProperSubset with swapped arguments.
// Space complexity: O(1) - no additional space required.
//
// Example:
//
//	s1 := Of(1, 2, 3)
//	s2 := Of(1, 2)
//	result := IsProperSuperset(s1, s2)  // returns true
func IsProperSuperset[T comparable](s1, s2 Set[T]) bool {
	return IsProperSubset(s2, s1)
}

// IsDisjoint checks if two sets have no elements in common.
// Returns true if the intersection of s1 and s2 is empty.
//
// Time complexity: O(min(|s1|, |s2|)) - optimized by iterating over the smaller set.
// Space complexity: O(1) - no additional space required.
//
// The function is optimized to iterate over the smaller set and check
// membership in the larger set, minimizing the number of operations.
//
// Example:
//
//	s1 := Of(1, 2, 3)
//	s2 := Of(4, 5, 6)
//	result := IsDisjoint(s1, s2)  // returns true
func IsDisjoint[T comparable](s1, s2 Set[T]) bool {
	// Choose the smaller set for iteration to minimize operations
	smaller, larger := s1, s2
	if s2.Size() < s1.Size() {
		smaller, larger = s2, s1
	}

	// Check if any element from the smaller set exists in the larger set
	for x := range smaller.Iter() {
		if larger.Contains(x) {
			return false // Found a common element
		}
	}

	return true // No common elements found
}

type Pair[T, U comparable] struct {
	First  T
	Second U
}

func (p Pair[T, U]) String() string {
	return fmt.Sprintf("(%v, %v)", p.First, p.Second)
}

// CartesianProduct computes the Cartesian product of two sets.
// Returns a set of Pair[T, U] where the first element comes from s1
// and the second element comes from s2.
//
// Time complexity: O(|s1| × |s2|) - must examine every pair combination.
// Space complexity: O(|s1| × |s2|) - stores all pair combinations.
//
// The result uses type-safe Pair[T, U] structs to store pairs, ensuring
// proper type safety and correct deduplication behavior. Each pair maintains
// the original types of elements from both input sets.
//
// Note: This operation can produce very large result sets. For sets of size
// n and m, the result will have n×m elements.
//
// Example:
//
//	s1 := Of(1, 2)
//	s2 := Of("a", "b")
//	result := CartesianProduct(s1, s2)  // contains {Pair{1,"a"}, Pair{1,"b"}, Pair{2,"a"}, Pair{2,"b"}}
//
//	// Accessing elements:
//	for pair := range result.Iter() {
//		fmt.Printf("First: %v, Second: %v\n", pair.First, pair.Second)
//	}
func CartesianProduct[T, U comparable](s1 Set[T], s2 Set[U]) Set[Pair[T, U]] {
	result := NewHashSet[Pair[T, U]](s1.Size() * s2.Size())

	for x := range s1.Iter() {
		for y := range s2.Iter() {
			result.Add(Pair[T, U]{First: x, Second: y})
		}
	}
	return result
}

// Of creates a new set containing the specified elements.
// This is a convenience function for creating sets from a list of values.
// Duplicate values in the input are automatically deduplicated.
//
// Time complexity: O(n) where n is the number of input items.
// Space complexity: O(k) where k is the number of unique items.
//
// The function pre-allocates capacity based on the input length for
// optimal performance, though the actual result size may be smaller
// if there are duplicate values in the input.
//
// Example:
//
//	set := Of(1, 2, 3, 2, 1)  // creates a set containing {1, 2, 3}
//	empty := Of[string]()     // creates an empty string set
func Of[T comparable](items ...T) Set[T] {
	// Pre-allocate capacity based on input length
	result := NewHashSet[T](len(items))

	// Add all items (duplicates are automatically handled)
	for _, item := range items {
		result.Add(item)
	}

	return result
}
