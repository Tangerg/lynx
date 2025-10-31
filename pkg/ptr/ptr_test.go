package ptr

import (
	"reflect"
	"testing"
)

// TestPointer tests the Pointer function
func TestPointer(t *testing.T) {
	t.Run("int pointer", func(t *testing.T) {
		value := 42
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if *ptr != value {
			t.Errorf("*Pointer(%d) = %d, want %d", value, *ptr, value)
		}
	})

	t.Run("string pointer", func(t *testing.T) {
		value := "hello"
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if *ptr != value {
			t.Errorf("*Pointer(%q) = %q, want %q", value, *ptr, value)
		}
	})

	t.Run("bool pointer", func(t *testing.T) {
		value := true
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if *ptr != value {
			t.Errorf("*Pointer(%v) = %v, want %v", value, *ptr, value)
		}
	})

	t.Run("float64 pointer", func(t *testing.T) {
		value := 3.14
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if *ptr != value {
			t.Errorf("*Pointer(%f) = %f, want %f", value, *ptr, value)
		}
	})

	t.Run("struct pointer", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		value := Person{Name: "Alice", Age: 30}
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if !reflect.DeepEqual(*ptr, value) {
			t.Errorf("*Pointer() = %+v, want %+v", *ptr, value)
		}
	})

	t.Run("slice pointer", func(t *testing.T) {
		value := []int{1, 2, 3}
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if !reflect.DeepEqual(*ptr, value) {
			t.Errorf("*Pointer() = %v, want %v", *ptr, value)
		}
	})

	t.Run("map pointer", func(t *testing.T) {
		value := map[string]int{"a": 1, "b": 2}
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if !reflect.DeepEqual(*ptr, value) {
			t.Errorf("*Pointer() = %v, want %v", *ptr, value)
		}
	})
}

// TestPointer_ZeroValues tests Pointer with zero values
func TestPointer_ZeroValues(t *testing.T) {
	t.Run("zero int", func(t *testing.T) {
		value := 0
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil for zero value")
		}

		if *ptr != 0 {
			t.Errorf("*Pointer(0) = %d, want 0", *ptr)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		value := ""
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil for empty string")
		}

		if *ptr != "" {
			t.Errorf("*Pointer(\"\") = %q, want \"\"", *ptr)
		}
	})

	t.Run("false bool", func(t *testing.T) {
		value := false
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil for false")
		}

		if *ptr != false {
			t.Errorf("*Pointer(false) = %v, want false", *ptr)
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		var value []int
		ptr := Pointer(value)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		if *ptr != nil {
			t.Errorf("*Pointer(nil slice) = %v, want nil", *ptr)
		}
	})
}

// TestPointer_Independence tests that pointer changes don't affect original
func TestPointer_Independence(t *testing.T) {
	t.Run("modify through pointer doesn't affect original", func(t *testing.T) {
		original := 42
		ptr := Pointer(original)
		*ptr = 100

		if original != 42 {
			t.Errorf("original value changed to %d, want 42", original)
		}

		if *ptr != 100 {
			t.Errorf("pointer value = %d, want 100", *ptr)
		}
	})
}

// TestValue tests the Value function
func TestValue(t *testing.T) {
	t.Run("non-nil int pointer", func(t *testing.T) {
		value := 42
		ptr := &value
		got := Value(ptr)

		if got != value {
			t.Errorf("Value(%d) = %d, want %d", value, got, value)
		}
	})

	t.Run("nil int pointer", func(t *testing.T) {
		var ptr *int
		got := Value(ptr)

		if got != 0 {
			t.Errorf("Value(nil *int) = %d, want 0", got)
		}
	})

	t.Run("non-nil string pointer", func(t *testing.T) {
		value := "hello"
		ptr := &value
		got := Value(ptr)

		if got != value {
			t.Errorf("Value(%q) = %q, want %q", value, got, value)
		}
	})

	t.Run("nil string pointer", func(t *testing.T) {
		var ptr *string
		got := Value(ptr)

		if got != "" {
			t.Errorf("Value(nil *string) = %q, want \"\"", got)
		}
	})

	t.Run("non-nil bool pointer", func(t *testing.T) {
		value := true
		ptr := &value
		got := Value(ptr)

		if got != value {
			t.Errorf("Value(%v) = %v, want %v", value, got, value)
		}
	})

	t.Run("nil bool pointer", func(t *testing.T) {
		var ptr *bool
		got := Value(ptr)

		if got != false {
			t.Errorf("Value(nil *bool) = %v, want false", got)
		}
	})

	t.Run("non-nil struct pointer", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		value := Person{Name: "Bob", Age: 25}
		ptr := &value
		got := Value(ptr)

		if !reflect.DeepEqual(got, value) {
			t.Errorf("Value() = %+v, want %+v", got, value)
		}
	})

	t.Run("nil struct pointer", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		var ptr *Person
		got := Value(ptr)

		expected := Person{}
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("Value(nil *Person) = %+v, want %+v", got, expected)
		}
	})

	t.Run("non-nil slice pointer", func(t *testing.T) {
		value := []int{1, 2, 3}
		ptr := &value
		got := Value(ptr)

		if !reflect.DeepEqual(got, value) {
			t.Errorf("Value() = %v, want %v", got, value)
		}
	})

	t.Run("nil slice pointer", func(t *testing.T) {
		var ptr *[]int
		got := Value(ptr)

		if got != nil {
			t.Errorf("Value(nil *[]int) = %v, want nil", got)
		}
	})

	t.Run("non-nil map pointer", func(t *testing.T) {
		value := map[string]int{"a": 1}
		ptr := &value
		got := Value(ptr)

		if !reflect.DeepEqual(got, value) {
			t.Errorf("Value() = %v, want %v", got, value)
		}
	})

	t.Run("nil map pointer", func(t *testing.T) {
		var ptr *map[string]int
		got := Value(ptr)

		if got != nil {
			t.Errorf("Value(nil *map) = %v, want nil", got)
		}
	})
}

// TestValue_ZeroValues tests Value with pointers to zero values
func TestValue_ZeroValues(t *testing.T) {
	t.Run("pointer to zero int", func(t *testing.T) {
		zero := 0
		ptr := &zero
		got := Value(ptr)

		if got != 0 {
			t.Errorf("Value(&0) = %d, want 0", got)
		}
	})

	t.Run("pointer to empty string", func(t *testing.T) {
		empty := ""
		ptr := &empty
		got := Value(ptr)

		if got != "" {
			t.Errorf("Value(&\"\") = %q, want \"\"", got)
		}
	})

	t.Run("pointer to false", func(t *testing.T) {
		f := false
		ptr := &f
		got := Value(ptr)

		if got != false {
			t.Errorf("Value(&false) = %v, want false", got)
		}
	})
}

// TestClone tests the Clone function
func TestClone(t *testing.T) {
	t.Run("clone int pointer", func(t *testing.T) {
		value := 42
		original := &value
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if *cloned != *original {
			t.Errorf("*Clone() = %d, want %d", *cloned, *original)
		}

		// Verify they are different pointers
		if cloned == original {
			t.Error("Clone() returned same pointer, want different pointer")
		}
	})

	t.Run("clone string pointer", func(t *testing.T) {
		value := "hello"
		original := &value
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if *cloned != *original {
			t.Errorf("*Clone() = %q, want %q", *cloned, *original)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer, want different pointer")
		}
	})

	t.Run("clone nil pointer", func(t *testing.T) {
		var ptr *int
		cloned := Clone(ptr)

		if cloned != nil {
			t.Errorf("Clone(nil) = %v, want nil", cloned)
		}
	})

	t.Run("clone struct pointer", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		value := Person{Name: "Charlie", Age: 35}
		original := &value
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if !reflect.DeepEqual(*cloned, *original) {
			t.Errorf("*Clone() = %+v, want %+v", *cloned, *original)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer, want different pointer")
		}
	})

	t.Run("clone bool pointer", func(t *testing.T) {
		value := true
		original := &value
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if *cloned != *original {
			t.Errorf("*Clone() = %v, want %v", *cloned, *original)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer, want different pointer")
		}
	})
}

// TestClone_Independence tests that cloned pointer is independent
func TestClone_Independence(t *testing.T) {
	t.Run("modify clone doesn't affect original", func(t *testing.T) {
		value := 42
		original := &value
		cloned := Clone(original)

		*cloned = 100

		if *original != 42 {
			t.Errorf("original value changed to %d, want 42", *original)
		}

		if *cloned != 100 {
			t.Errorf("cloned value = %d, want 100", *cloned)
		}
	})

	t.Run("modify original doesn't affect clone", func(t *testing.T) {
		value := "hello"
		original := &value
		cloned := Clone(original)

		*original = "world"

		if *cloned != "hello" {
			t.Errorf("cloned value changed to %q, want \"hello\"", *cloned)
		}

		if *original != "world" {
			t.Errorf("original value = %q, want \"world\"", *original)
		}
	})
}

// TestClone_SliceAndMap tests Clone with slices and maps
func TestClone_SliceAndMap(t *testing.T) {
	t.Run("clone slice pointer", func(t *testing.T) {
		value := []int{1, 2, 3}
		original := &value
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if !reflect.DeepEqual(*cloned, *original) {
			t.Errorf("*Clone() = %v, want %v", *cloned, *original)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer, want different pointer")
		}

		// Note: slice elements share the same backing array
		// This is expected behavior for shallow copy
	})

	t.Run("clone map pointer", func(t *testing.T) {
		value := map[string]int{"a": 1, "b": 2}
		original := &value
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if !reflect.DeepEqual(*cloned, *original) {
			t.Errorf("*Clone() = %v, want %v", *cloned, *original)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer, want different pointer")
		}

		// Note: map reference is shared
		// This is expected behavior for shallow copy
	})
}

// TestClone_ZeroValues tests Clone with zero values
func TestClone_ZeroValues(t *testing.T) {
	t.Run("clone pointer to zero int", func(t *testing.T) {
		zero := 0
		original := &zero
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if *cloned != 0 {
			t.Errorf("*Clone(&0) = %d, want 0", *cloned)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer")
		}
	})

	t.Run("clone pointer to empty string", func(t *testing.T) {
		empty := ""
		original := &empty
		cloned := Clone(original)

		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if *cloned != "" {
			t.Errorf("*Clone(&\"\") = %q, want \"\"", *cloned)
		}

		if cloned == original {
			t.Error("Clone() returned same pointer")
		}
	})
}

// TestIntegration tests combined usage of all functions
func TestIntegration(t *testing.T) {
	t.Run("Pointer -> Value round trip", func(t *testing.T) {
		original := 42
		ptr := Pointer(original)
		value := Value(ptr)

		if value != original {
			t.Errorf("round trip failed: got %d, want %d", value, original)
		}
	})

	t.Run("Pointer -> Clone -> Value", func(t *testing.T) {
		original := "test"
		ptr := Pointer(original)
		cloned := Clone(ptr)
		value := Value(cloned)

		if value != original {
			t.Errorf("got %q, want %q", value, original)
		}
	})

	t.Run("nil handling chain", func(t *testing.T) {
		var ptr *int
		cloned := Clone(ptr)
		value := Value(cloned)

		if cloned != nil {
			t.Error("Clone(nil) should return nil")
		}

		if value != 0 {
			t.Errorf("Value(nil) = %d, want 0", value)
		}
	})
}

// TestComplexTypes tests with more complex types
func TestComplexTypes(t *testing.T) {
	type Address struct {
		Street string
		City   string
	}

	type Person struct {
		Name    string
		Age     int
		Address Address
	}

	t.Run("nested struct pointer", func(t *testing.T) {
		person := Person{
			Name: "Alice",
			Age:  30,
			Address: Address{
				Street: "123 Main St",
				City:   "Wonderland",
			},
		}

		ptr := Pointer(person)
		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		cloned := Clone(ptr)
		if cloned == nil {
			t.Fatal("Clone() returned nil")
		}

		if !reflect.DeepEqual(*cloned, person) {
			t.Errorf("cloned struct mismatch")
		}

		value := Value(cloned)
		if !reflect.DeepEqual(value, person) {
			t.Errorf("Value() struct mismatch")
		}
	})

	t.Run("pointer to interface", func(t *testing.T) {
		var i interface{} = 42
		ptr := Pointer(i)

		if ptr == nil {
			t.Fatal("Pointer() returned nil")
		}

		value := Value(ptr)
		if value != i {
			t.Errorf("Value() = %v, want %v", value, i)
		}
	})
}

// BenchmarkPointer benchmarks the Pointer function
func BenchmarkPointer(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Pointer(42)
		}
	})

	b.Run("string", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Pointer("hello")
		}
	})

	b.Run("struct", func(b *testing.B) {
		type Person struct {
			Name string
			Age  int
		}
		person := Person{Name: "Alice", Age: 30}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Pointer(person)
		}
	})
}

// BenchmarkValue benchmarks the Value function
func BenchmarkValue(b *testing.B) {
	value := 42
	ptr := &value

	b.Run("non-nil", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Value(ptr)
		}
	})

	b.Run("nil", func(b *testing.B) {
		var nilPtr *int
		for i := 0; i < b.N; i++ {
			_ = Value(nilPtr)
		}
	})
}

// BenchmarkClone benchmarks the Clone function
func BenchmarkClone(b *testing.B) {
	value := 42
	ptr := &value

	b.Run("non-nil", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Clone(ptr)
		}
	})

	b.Run("nil", func(b *testing.B) {
		var nilPtr *int
		for i := 0; i < b.N; i++ {
			_ = Clone(nilPtr)
		}
	})

	b.Run("struct", func(b *testing.B) {
		type Person struct {
			Name string
			Age  int
		}
		person := Person{Name: "Bob", Age: 25}
		ptr := &person
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Clone(ptr)
		}
	})
}
