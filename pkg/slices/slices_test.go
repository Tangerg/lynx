package slices

import (
	"reflect"
	"testing"
)

// TestEnsureIndex tests the EnsureIndex function
func TestEnsureIndex(t *testing.T) {
	t.Run("index within length", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5}
		result := EnsureIndex(s, 2)

		if len(result) != len(s) {
			t.Errorf("len(result) = %d, want %d", len(result), len(s))
		}

		if !reflect.DeepEqual(result, s) {
			t.Errorf("result = %v, want %v", result, s)
		}
	})

	t.Run("index within capacity", func(t *testing.T) {
		s := make([]int, 3, 10)
		s[0], s[1], s[2] = 1, 2, 3

		result := EnsureIndex(s, 5)

		if len(result) != 6 {
			t.Errorf("len(result) = %d, want 6", len(result))
		}

		if cap(result) != 10 {
			t.Errorf("cap(result) = %d, want 10", cap(result))
		}

		// Check original values preserved
		if result[0] != 1 || result[1] != 2 || result[2] != 3 {
			t.Errorf("original values not preserved: %v", result[:3])
		}

		// Check new elements are zero values
		for i := 3; i < len(result); i++ {
			if result[i] != 0 {
				t.Errorf("result[%d] = %d, want 0", i, result[i])
			}
		}
	})

	t.Run("index exceeds capacity", func(t *testing.T) {
		s := []int{1, 2, 3}
		result := EnsureIndex(s, 10)

		if len(result) != 11 {
			t.Errorf("len(result) = %d, want 11", len(result))
		}

		if cap(result) < 11 {
			t.Errorf("cap(result) = %d, want at least 11", cap(result))
		}

		// Check original values preserved
		if result[0] != 1 || result[1] != 2 || result[2] != 3 {
			t.Errorf("original values not preserved: %v", result[:3])
		}

		// Check new elements are zero values
		for i := 3; i < len(result); i++ {
			if result[i] != 0 {
				t.Errorf("result[%d] = %d, want 0", i, result[i])
			}
		}
	})

	t.Run("index zero", func(t *testing.T) {
		s := []int{1, 2, 3}
		result := EnsureIndex(s, 0)

		if len(result) != len(s) {
			t.Errorf("len(result) = %d, want %d", len(result), len(s))
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		var s []int
		result := EnsureIndex(s, 5)

		if len(result) != 6 {
			t.Errorf("len(result) = %d, want 6", len(result))
		}

		for i := 0; i < len(result); i++ {
			if result[i] != 0 {
				t.Errorf("result[%d] = %d, want 0", i, result[i])
			}
		}
	})

	t.Run("negative index panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("EnsureIndex should panic on negative index")
			} else {
				if msg, ok := r.(string); ok {
					expected := "index must be positive"
					if msg != expected {
						t.Errorf("panic message = %q, want %q", msg, expected)
					}
				}
			}
		}()

		s := []int{1, 2, 3}
		_ = EnsureIndex(s, -1)
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"a", "b", "c"}
		result := EnsureIndex(s, 5)

		if len(result) != 6 {
			t.Errorf("len(result) = %d, want 6", len(result))
		}

		if result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("original values not preserved")
		}

		for i := 3; i < len(result); i++ {
			if result[i] != "" {
				t.Errorf("result[%d] = %q, want empty string", i, result[i])
			}
		}
	})

	t.Run("struct slice", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		s := []Person{{Name: "Alice", Age: 30}}
		result := EnsureIndex(s, 3)

		if len(result) != 4 {
			t.Errorf("len(result) = %d, want 4", len(result))
		}

		if result[0].Name != "Alice" || result[0].Age != 30 {
			t.Errorf("original value not preserved")
		}

		// Check zero values
		for i := 1; i < len(result); i++ {
			if result[i].Name != "" || result[i].Age != 0 {
				t.Errorf("result[%d] not zero value", i)
			}
		}
	})

	t.Run("large index", func(t *testing.T) {
		s := []int{1}
		result := EnsureIndex(s, 1000)

		if len(result) != 1001 {
			t.Errorf("len(result) = %d, want 1001", len(result))
		}

		if result[0] != 1 {
			t.Error("original value not preserved")
		}
	})
}

// TestChunk tests the Chunk function
func TestChunk(t *testing.T) {
	t.Run("evenly divisible", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5, 6}
		result := Chunk(s, 2)

		expected := [][]int{{1, 2}, {3, 4}, {5, 6}}

		if len(result) != len(expected) {
			t.Errorf("len(result) = %d, want %d", len(result), len(expected))
		}

		for i, chunk := range result {
			if !reflect.DeepEqual(chunk, expected[i]) {
				t.Errorf("result[%d] = %v, want %v", i, chunk, expected[i])
			}
		}
	})

	t.Run("not evenly divisible", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5}
		result := Chunk(s, 2)

		expected := [][]int{{1, 2}, {3, 4}, {5}}

		if len(result) != len(expected) {
			t.Errorf("len(result) = %d, want %d", len(result), len(expected))
		}

		for i, chunk := range result {
			if !reflect.DeepEqual(chunk, expected[i]) {
				t.Errorf("result[%d] = %v, want %v", i, chunk, expected[i])
			}
		}
	})

	t.Run("chunk size larger than slice", func(t *testing.T) {
		s := []int{1, 2, 3}
		result := Chunk(s, 10)

		if len(result) != 1 {
			t.Errorf("len(result) = %d, want 1", len(result))
		}

		if !reflect.DeepEqual(result[0], s) {
			t.Errorf("result[0] = %v, want %v", result[0], s)
		}
	})

	t.Run("chunk size equals slice length", func(t *testing.T) {
		s := []int{1, 2, 3}
		result := Chunk(s, 3)

		if len(result) != 1 {
			t.Errorf("len(result) = %d, want 1", len(result))
		}

		if !reflect.DeepEqual(result[0], s) {
			t.Errorf("result[0] = %v, want %v", result[0], s)
		}
	})

	t.Run("chunk size of 1", func(t *testing.T) {
		s := []int{1, 2, 3}
		result := Chunk(s, 1)

		expected := [][]int{{1}, {2}, {3}}

		if len(result) != len(expected) {
			t.Errorf("len(result) = %d, want %d", len(result), len(expected))
		}

		for i, chunk := range result {
			if !reflect.DeepEqual(chunk, expected[i]) {
				t.Errorf("result[%d] = %v, want %v", i, chunk, expected[i])
			}
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		var s []int
		result := Chunk(s, 3)

		if len(result) != 0 {
			t.Errorf("len(result) = %d, want 0", len(result))
		}
	})

	t.Run("zero chunk size panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Chunk should panic on zero size")
			} else {
				if msg, ok := r.(string); ok {
					expected := "chunk size must be positive"
					if msg != expected {
						t.Errorf("panic message = %q, want %q", msg, expected)
					}
				}
			}
		}()

		s := []int{1, 2, 3}
		_ = Chunk(s, 0)
	})

	t.Run("negative chunk size panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Chunk should panic on negative size")
			}
		}()

		s := []int{1, 2, 3}
		_ = Chunk(s, -1)
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"a", "b", "c", "d", "e"}
		result := Chunk(s, 2)

		expected := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}

		if !reflect.DeepEqual(result, expected) {
			t.Errorf("result = %v, want %v", result, expected)
		}
	})

	t.Run("capacity of chunks", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5, 6}
		result := Chunk(s, 2)

		// Each chunk should have capacity equal to its length
		// to prevent accidental modification
		for i, chunk := range result {
			if cap(chunk) != len(chunk) {
				t.Errorf("chunk[%d] cap = %d, want %d", i, cap(chunk), len(chunk))
			}
		}
	})

	t.Run("modification isolation", func(t *testing.T) {
		s := []int{1, 2, 3, 4}
		result := Chunk(s, 2)

		// Modify a chunk
		result[0][0] = 999

		// Original slice should be modified (shares backing array)
		if s[0] != 999 {
			t.Error("chunk modification should affect original slice")
		}

		// But can't append beyond chunk capacity
		// This is prevented by three-index slicing in Chunk
	})
}

// TestAt tests the At function
func TestAt(t *testing.T) {
	t.Run("positive index in range", func(t *testing.T) {
		s := []int{10, 20, 30, 40, 50}

		testCases := []struct {
			index    int
			expected int
		}{
			{0, 10},
			{1, 20},
			{2, 30},
			{3, 40},
			{4, 50},
		}

		for _, tc := range testCases {
			val, ok := At(s, tc.index)
			if !ok {
				t.Errorf("At(%d) ok = false, want true", tc.index)
			}
			if val != tc.expected {
				t.Errorf("At(%d) = %d, want %d", tc.index, val, tc.expected)
			}
		}
	})

	t.Run("negative index", func(t *testing.T) {
		s := []int{10, 20, 30, 40, 50}

		testCases := []struct {
			index    int
			expected int
		}{
			{-1, 50}, // last element
			{-2, 40}, // second to last
			{-3, 30},
			{-4, 20},
			{-5, 10}, // first element
		}

		for _, tc := range testCases {
			val, ok := At(s, tc.index)
			if !ok {
				t.Errorf("At(%d) ok = false, want true", tc.index)
			}
			if val != tc.expected {
				t.Errorf("At(%d) = %d, want %d", tc.index, val, tc.expected)
			}
		}
	})

	t.Run("out of bounds positive", func(t *testing.T) {
		s := []int{10, 20, 30}
		val, ok := At(s, 10)

		if ok {
			t.Error("At(10) ok = true, want false")
		}
		if val != 0 {
			t.Errorf("At(10) = %d, want 0 (zero value)", val)
		}
	})

	t.Run("out of bounds negative", func(t *testing.T) {
		s := []int{10, 20, 30}
		val, ok := At(s, -10)

		if ok {
			t.Error("At(-10) ok = true, want false")
		}
		if val != 0 {
			t.Errorf("At(-10) = %d, want 0 (zero value)", val)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		var s []int
		val, ok := At(s, 0)

		if ok {
			t.Error("At(0) on empty slice ok = true, want false")
		}
		if val != 0 {
			t.Errorf("At(0) = %d, want 0", val)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"a", "b", "c"}
		val, ok := At(s, 1)

		if !ok {
			t.Error("At(1) ok = false, want true")
		}
		if val != "b" {
			t.Errorf("At(1) = %q, want \"b\"", val)
		}

		val, ok = At(s, -1)
		if !ok {
			t.Error("At(-1) ok = false, want true")
		}
		if val != "c" {
			t.Errorf("At(-1) = %q, want \"c\"", val)
		}
	})

	t.Run("single element slice", func(t *testing.T) {
		s := []int{42}

		val, ok := At(s, 0)
		if !ok || val != 42 {
			t.Errorf("At(0) = %d, %v; want 42, true", val, ok)
		}

		val, ok = At(s, -1)
		if !ok || val != 42 {
			t.Errorf("At(-1) = %d, %v; want 42, true", val, ok)
		}

		val, ok = At(s, 1)
		if ok {
			t.Error("At(1) ok = true, want false")
		}
	})
}

// TestAtOr tests the AtOr function
func TestAtOr(t *testing.T) {
	t.Run("valid positive index", func(t *testing.T) {
		s := []int{10, 20, 30}
		val := AtOr(s, 1, -1)

		if val != 20 {
			t.Errorf("AtOr(1, -1) = %d, want 20", val)
		}
	})

	t.Run("valid negative index", func(t *testing.T) {
		s := []int{10, 20, 30}
		val := AtOr(s, -1, -1)

		if val != 30 {
			t.Errorf("AtOr(-1, -1) = %d, want 30", val)
		}
	})

	t.Run("invalid index returns default", func(t *testing.T) {
		s := []int{10, 20, 30}
		val := AtOr(s, 10, -1)

		if val != -1 {
			t.Errorf("AtOr(10, -1) = %d, want -1", val)
		}
	})

	t.Run("empty slice returns default", func(t *testing.T) {
		var s []int
		val := AtOr(s, 0, 999)

		if val != 999 {
			t.Errorf("AtOr(0, 999) = %d, want 999", val)
		}
	})

	t.Run("string slice with default", func(t *testing.T) {
		s := []string{"a", "b", "c"}

		val := AtOr(s, 1, "default")
		if val != "b" {
			t.Errorf("AtOr(1, \"default\") = %q, want \"b\"", val)
		}

		val = AtOr(s, 10, "default")
		if val != "default" {
			t.Errorf("AtOr(10, \"default\") = %q, want \"default\"", val)
		}
	})

	t.Run("zero default value", func(t *testing.T) {
		s := []int{10, 20, 30}
		val := AtOr(s, 10, 0)

		if val != 0 {
			t.Errorf("AtOr(10, 0) = %d, want 0", val)
		}
	})
}

// TestFirst tests the First function
func TestFirst(t *testing.T) {
	t.Run("non-empty slice", func(t *testing.T) {
		s := []int{10, 20, 30}
		val, ok := First(s)

		if !ok {
			t.Error("First() ok = false, want true")
		}
		if val != 10 {
			t.Errorf("First() = %d, want 10", val)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		var s []int
		val, ok := First(s)

		if ok {
			t.Error("First() ok = true, want false")
		}
		if val != 0 {
			t.Errorf("First() = %d, want 0", val)
		}
	})

	t.Run("single element", func(t *testing.T) {
		s := []int{42}
		val, ok := First(s)

		if !ok || val != 42 {
			t.Errorf("First() = %d, %v; want 42, true", val, ok)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"first", "second", "third"}
		val, ok := First(s)

		if !ok || val != "first" {
			t.Errorf("First() = %q, %v; want \"first\", true", val, ok)
		}
	})
}

// TestFirstOr tests the FirstOr function
func TestFirstOr(t *testing.T) {
	t.Run("non-empty slice", func(t *testing.T) {
		s := []int{10, 20, 30}
		val := FirstOr(s, -1)

		if val != 10 {
			t.Errorf("FirstOr() = %d, want 10", val)
		}
	})

	t.Run("empty slice returns default", func(t *testing.T) {
		var s []int
		val := FirstOr(s, -1)

		if val != -1 {
			t.Errorf("FirstOr() = %d, want -1", val)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"first", "second"}
		val := FirstOr(s, "default")

		if val != "first" {
			t.Errorf("FirstOr() = %q, want \"first\"", val)
		}

		var empty []string
		val = FirstOr(empty, "default")
		if val != "default" {
			t.Errorf("FirstOr() = %q, want \"default\"", val)
		}
	})
}

// TestLast tests the Last function
func TestLast(t *testing.T) {
	t.Run("non-empty slice", func(t *testing.T) {
		s := []int{10, 20, 30}
		val, ok := Last(s)

		if !ok {
			t.Error("Last() ok = false, want true")
		}
		if val != 30 {
			t.Errorf("Last() = %d, want 30", val)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		var s []int
		val, ok := Last(s)

		if ok {
			t.Error("Last() ok = true, want false")
		}
		if val != 0 {
			t.Errorf("Last() = %d, want 0", val)
		}
	})

	t.Run("single element", func(t *testing.T) {
		s := []int{42}
		val, ok := Last(s)

		if !ok || val != 42 {
			t.Errorf("Last() = %d, %v; want 42, true", val, ok)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"first", "second", "last"}
		val, ok := Last(s)

		if !ok || val != "last" {
			t.Errorf("Last() = %q, %v; want \"last\", true", val, ok)
		}
	})
}

// TestLastOr tests the LastOr function
func TestLastOr(t *testing.T) {
	t.Run("non-empty slice", func(t *testing.T) {
		s := []int{10, 20, 30}
		val := LastOr(s, -1)

		if val != 30 {
			t.Errorf("LastOr() = %d, want 30", val)
		}
	})

	t.Run("empty slice returns default", func(t *testing.T) {
		var s []int
		val := LastOr(s, -1)

		if val != -1 {
			t.Errorf("LastOr() = %d, want -1", val)
		}
	})

	t.Run("string slice", func(t *testing.T) {
		s := []string{"first", "last"}
		val := LastOr(s, "default")

		if val != "last" {
			t.Errorf("LastOr() = %q, want \"last\"", val)
		}

		var empty []string
		val = LastOr(empty, "default")
		if val != "default" {
			t.Errorf("LastOr() = %q, want \"default\"", val)
		}
	})
}

// TestComplexTypes tests functions with complex types
func TestComplexTypes(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}

	t.Run("struct slice with At", func(t *testing.T) {
		people := []Person{
			{"Alice", 30},
			{"Bob", 25},
			{"Charlie", 35},
		}

		person, ok := At(people, 1)
		if !ok || person.Name != "Bob" {
			t.Errorf("At(1) = %+v, %v; want Bob", person, ok)
		}

		person, ok = At(people, -1)
		if !ok || person.Name != "Charlie" {
			t.Errorf("At(-1) = %+v, %v; want Charlie", person, ok)
		}
	})

	t.Run("struct slice with FirstOr", func(t *testing.T) {
		people := []Person{{"Alice", 30}}
		defaultPerson := Person{"Default", 0}

		person := FirstOr(people, defaultPerson)
		if person.Name != "Alice" {
			t.Errorf("FirstOr() = %+v, want Alice", person)
		}

		var empty []Person
		person = FirstOr(empty, defaultPerson)
		if person.Name != "Default" {
			t.Errorf("FirstOr() = %+v, want Default", person)
		}
	})
}

// BenchmarkEnsureIndex benchmarks EnsureIndex function
func BenchmarkEnsureIndex(b *testing.B) {
	b.Run("within length", func(b *testing.B) {
		s := make([]int, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = EnsureIndex(s, 50)
		}
	})

	b.Run("within capacity", func(b *testing.B) {
		s := make([]int, 50, 100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = EnsureIndex(s, 75)
		}
	})

	b.Run("needs reallocation", func(b *testing.B) {
		s := make([]int, 10)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = EnsureIndex(s, 100)
		}
	})
}

// BenchmarkChunk benchmarks Chunk function
func BenchmarkChunk(b *testing.B) {
	s := make([]int, 1000)
	for i := range s {
		s[i] = i
	}

	b.Run("size 10", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Chunk(s, 10)
		}
	})

	b.Run("size 100", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Chunk(s, 100)
		}
	})
}

// BenchmarkAt benchmarks At function
func BenchmarkAt(b *testing.B) {
	s := make([]int, 1000)

	b.Run("positive index", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = At(s, 500)
		}
	})

	b.Run("negative index", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = At(s, -1)
		}
	})
}
