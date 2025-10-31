package result

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestNew tests the New constructor
func TestNew(t *testing.T) {
	t.Run("with value and no error", func(t *testing.T) {
		result := New(42, nil)

		if result == nil {
			t.Fatal("New() returned nil")
		}

		v, err := result.Get()
		if v != 42 {
			t.Errorf("Get() value = %d, want 42", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("with value and error", func(t *testing.T) {
		testErr := errors.New("test error")
		result := New(42, testErr)

		v, err := result.Get()
		if v != 42 {
			t.Errorf("Get() value = %d, want 42", v)
		}
		if err != testErr {
			t.Errorf("Get() error = %v, want %v", err, testErr)
		}
	})

	t.Run("with zero value and error", func(t *testing.T) {
		testErr := errors.New("test error")
		result := New(0, testErr)

		v, err := result.Get()
		if v != 0 {
			t.Errorf("Get() value = %d, want 0", v)
		}
		if err != testErr {
			t.Errorf("Get() error = %v, want %v", err, testErr)
		}
	})

	t.Run("with string type", func(t *testing.T) {
		result := New("hello", nil)

		v, err := result.Get()
		if v != "hello" {
			t.Errorf("Get() value = %q, want \"hello\"", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("with struct type", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		person := Person{Name: "Alice", Age: 30}
		result := New(person, nil)

		v, err := result.Get()
		if !reflect.DeepEqual(v, person) {
			t.Errorf("Get() value = %+v, want %+v", v, person)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})
}

// TestValue tests the Value constructor
func TestValue(t *testing.T) {
	t.Run("int value", func(t *testing.T) {
		result := Value(42)

		if result == nil {
			t.Fatal("Value() returned nil")
		}

		v, err := result.Get()
		if v != 42 {
			t.Errorf("Get() value = %d, want 42", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("string value", func(t *testing.T) {
		result := Value("test")

		v, err := result.Get()
		if v != "test" {
			t.Errorf("Get() value = %q, want \"test\"", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("bool value", func(t *testing.T) {
		result := Value(true)

		v, err := result.Get()
		if v != true {
			t.Errorf("Get() value = %v, want true", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("zero value", func(t *testing.T) {
		result := Value(0)

		v, err := result.Get()
		if v != 0 {
			t.Errorf("Get() value = %d, want 0", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("nil slice", func(t *testing.T) {
		var slice []int
		result := Value(slice)

		v, err := result.Get()
		if v != nil {
			t.Errorf("Get() value = %v, want nil", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("pointer value", func(t *testing.T) {
		val := 42
		ptr := &val
		result := Value(ptr)

		v, err := result.Get()
		if v != ptr {
			t.Errorf("Get() value = %v, want %v", v, ptr)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})
}

// TestError tests the Error constructor
func TestError(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[int](testErr)

		if result == nil {
			t.Fatal("Error() returned nil")
		}

		v, err := result.Get()
		if v != 0 {
			t.Errorf("Get() value = %d, want 0 (zero value)", v)
		}
		if err != testErr {
			t.Errorf("Get() error = %v, want %v", err, testErr)
		}
	})

	t.Run("with nil error", func(t *testing.T) {
		result := Error[string](nil)

		v, err := result.Get()
		if v != "" {
			t.Errorf("Get() value = %q, want \"\" (zero value)", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("different types", func(t *testing.T) {
		testErr := errors.New("test error")

		intResult := Error[int](testErr)
		v1, err1 := intResult.Get()
		if v1 != 0 || err1 != testErr {
			t.Errorf("int result: value = %d, err = %v", v1, err1)
		}

		stringResult := Error[string](testErr)
		v2, err2 := stringResult.Get()
		if v2 != "" || err2 != testErr {
			t.Errorf("string result: value = %q, err = %v", v2, err2)
		}

		boolResult := Error[bool](testErr)
		v3, err3 := boolResult.Get()
		if v3 != false || err3 != testErr {
			t.Errorf("bool result: value = %v, err = %v", v3, err3)
		}
	})
}

// TestResult_Get tests the Get method
func TestResult_Get(t *testing.T) {
	t.Run("successful result", func(t *testing.T) {
		result := Value(42)
		v, err := result.Get()

		if v != 42 {
			t.Errorf("Get() value = %d, want 42", v)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("error result", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[int](testErr)
		v, err := result.Get()

		if v != 0 {
			t.Errorf("Get() value = %d, want 0", v)
		}
		if err != testErr {
			t.Errorf("Get() error = %v, want %v", err, testErr)
		}
	})

	t.Run("mixed result", func(t *testing.T) {
		testErr := errors.New("test error")
		result := New(42, testErr)
		v, err := result.Get()

		if v != 42 {
			t.Errorf("Get() value = %d, want 42", v)
		}
		if err != testErr {
			t.Errorf("Get() error = %v, want %v", err, testErr)
		}
	})
}

// TestResult_Error tests the Error method
func TestResult_Error(t *testing.T) {
	t.Run("successful result", func(t *testing.T) {
		result := Value(42)
		err := result.Error()

		if err != nil {
			t.Errorf("Error() = %v, want nil", err)
		}
	})

	t.Run("error result", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[int](testErr)
		err := result.Error()

		if err != testErr {
			t.Errorf("Error() = %v, want %v", err, testErr)
		}
	})

	t.Run("nil error", func(t *testing.T) {
		result := New(42, nil)
		err := result.Error()

		if err != nil {
			t.Errorf("Error() = %v, want nil", err)
		}
	})
}

// TestResult_Value tests the Value method
func TestResult_Value(t *testing.T) {
	t.Run("successful result", func(t *testing.T) {
		result := Value(42)
		v := result.Value()

		if v != 42 {
			t.Errorf("Value() = %d, want 42", v)
		}
	})

	t.Run("error result returns zero value", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[int](testErr)
		v := result.Value()

		if v != 0 {
			t.Errorf("Value() = %d, want 0 (zero value)", v)
		}
	})

	t.Run("string zero value", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[string](testErr)
		v := result.Value()

		if v != "" {
			t.Errorf("Value() = %q, want \"\" (zero value)", v)
		}
	})

	t.Run("bool zero value", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[bool](testErr)
		v := result.Value()

		if v != false {
			t.Errorf("Value() = %v, want false (zero value)", v)
		}
	})
}

// TestResult_String tests the String method
func TestResult_String(t *testing.T) {
	t.Run("successful int result", func(t *testing.T) {
		result := Value(42)
		s := result.String()

		expected := "value: 42"
		if s != expected {
			t.Errorf("String() = %q, want %q", s, expected)
		}
	})

	t.Run("successful string result", func(t *testing.T) {
		result := Value("hello")
		s := result.String()

		expected := "value: hello"
		if s != expected {
			t.Errorf("String() = %q, want %q", s, expected)
		}
	})

	t.Run("error result", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[int](testErr)
		s := result.String()

		expected := "error: test error"
		if s != expected {
			t.Errorf("String() = %q, want %q", s, expected)
		}
	})

	t.Run("struct with Stringer interface", func(t *testing.T) {
		type Person struct {
			Name string
		}
		// Implement fmt.Stringer
		type StringerPerson struct {
			Person
		}
		sp := StringerPerson{Person: Person{Name: "Alice"}}
		// Note: This would work if StringerPerson implements String() method
		result := Value(sp)
		s := result.String()

		// Should contain the struct representation
		if !strings.HasPrefix(s, "value: ") {
			t.Errorf("String() = %q, should start with 'value: '", s)
		}
	})

	t.Run("struct without Stringer", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		person := Person{Name: "Bob", Age: 25}
		result := Value(person)
		s := result.String()

		if !strings.HasPrefix(s, "value: ") {
			t.Errorf("String() = %q, should start with 'value: '", s)
		}
		if !strings.Contains(s, "Bob") {
			t.Errorf("String() = %q, should contain 'Bob'", s)
		}
	})

	t.Run("zero value result", func(t *testing.T) {
		result := Value(0)
		s := result.String()

		expected := "value: 0"
		if s != expected {
			t.Errorf("String() = %q, want %q", s, expected)
		}
	})
}

// TestMap tests the Map function
func TestMap(t *testing.T) {
	t.Run("map successful result", func(t *testing.T) {
		result := Value(10)
		mapped := Map(result, func(x int) int {
			return x * 2
		})

		v, err := mapped.Get()
		if v != 20 {
			t.Errorf("Map() value = %d, want 20", v)
		}
		if err != nil {
			t.Errorf("Map() error = %v, want nil", err)
		}
	})

	t.Run("map error result", func(t *testing.T) {
		testErr := errors.New("test error")
		result := Error[int](testErr)
		mapped := Map(result, func(x int) int {
			return x * 2
		})

		v, err := mapped.Get()
		if v != 0 {
			t.Errorf("Map() value = %d, want 0", v)
		}
		if err != testErr {
			t.Errorf("Map() error = %v, want %v", err, testErr)
		}
	})

	t.Run("map to different type", func(t *testing.T) {
		result := Value(42)
		mapped := Map(result, func(x int) string {
			return fmt.Sprintf("number: %d", x)
		})

		v, err := mapped.Get()
		expected := "number: 42"
		if v != expected {
			t.Errorf("Map() value = %q, want %q", v, expected)
		}
		if err != nil {
			t.Errorf("Map() error = %v, want nil", err)
		}
	})

	t.Run("map string to int", func(t *testing.T) {
		result := Value("hello")
		mapped := Map(result, func(s string) int {
			return len(s)
		})

		v, err := mapped.Get()
		if v != 5 {
			t.Errorf("Map() value = %d, want 5", v)
		}
		if err != nil {
			t.Errorf("Map() error = %v, want nil", err)
		}
	})

	t.Run("map struct", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		result := Value(Person{Name: "Alice", Age: 30})
		mapped := Map(result, func(p Person) string {
			return p.Name
		})

		v, err := mapped.Get()
		if v != "Alice" {
			t.Errorf("Map() value = %q, want \"Alice\"", v)
		}
		if err != nil {
			t.Errorf("Map() error = %v, want nil", err)
		}
	})

	t.Run("chained maps", func(t *testing.T) {
		result := Value(5)
		mapped1 := Map(result, func(x int) int { return x * 2 })
		mapped2 := Map(mapped1, func(x int) int { return x + 3 })
		mapped3 := Map(mapped2, func(x int) string { return fmt.Sprintf("%d", x) })

		v, err := mapped3.Get()
		expected := "13" // (5 * 2) + 3
		if v != expected {
			t.Errorf("chained Map() value = %q, want %q", v, expected)
		}
		if err != nil {
			t.Errorf("chained Map() error = %v, want nil", err)
		}
	})

	t.Run("error propagation through chain", func(t *testing.T) {
		testErr := errors.New("initial error")
		result := Error[int](testErr)
		mapped1 := Map(result, func(x int) int { return x * 2 })
		mapped2 := Map(mapped1, func(x int) string { return fmt.Sprintf("%d", x) })

		v, err := mapped2.Get()
		if v != "" {
			t.Errorf("Map() value = %q, want \"\" (zero value)", v)
		}
		if err != testErr {
			t.Errorf("Map() error = %v, want %v", err, testErr)
		}
	})
}

// TestResult_ComplexTypes tests with complex types
func TestResult_ComplexTypes(t *testing.T) {
	t.Run("slice type", func(t *testing.T) {
		slice := []int{1, 2, 3, 4, 5}
		result := Value(slice)

		v, err := result.Get()
		if !reflect.DeepEqual(v, slice) {
			t.Errorf("Get() = %v, want %v", v, slice)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("map type", func(t *testing.T) {
		m := map[string]int{"a": 1, "b": 2}
		result := Value(m)

		v, err := result.Get()
		if !reflect.DeepEqual(v, m) {
			t.Errorf("Get() = %v, want %v", v, m)
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("channel type", func(t *testing.T) {
		ch := make(chan int, 1)
		result := Value(ch)

		v, err := result.Get()
		if v != ch {
			t.Errorf("Get() channel mismatch")
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})

	t.Run("function type", func(t *testing.T) {
		fn := func(x int) int { return x * 2 }
		result := Value(fn)

		v, err := result.Get()
		if v == nil {
			t.Error("Get() function is nil")
		}
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
	})
}

// TestResult_NilResult tests behavior with nil Result pointer
func TestResult_NilResult(t *testing.T) {
	// Note: These tests would panic, so we use recover
	t.Run("nil result Get", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on nil Result.Get()")
			}
		}()
		var result *Result[int]
		_, _ = result.Get()
	})

	t.Run("nil result Error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on nil Result.Error()")
			}
		}()
		var result *Result[int]
		_ = result.Error()
	})

	t.Run("nil result Value", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on nil Result.Value()")
			}
		}()
		var result *Result[int]
		_ = result.Value()
	})
}

// TestResult_UsagePatterns tests common usage patterns
func TestResult_UsagePatterns(t *testing.T) {
	t.Run("wrapping existing function", func(t *testing.T) {
		// Simulate a function that returns (int, error)
		divide := func(a, b int) (int, error) {
			if b == 0 {
				return 0, errors.New("division by zero")
			}
			return a / b, nil
		}

		// Successful case
		result1 := New(divide(10, 2))
		v1, err1 := result1.Get()
		if v1 != 5 || err1 != nil {
			t.Errorf("divide(10, 2) = %d, %v; want 5, nil", v1, err1)
		}

		// Error case
		result2 := New(divide(10, 0))
		v2, err2 := result2.Get()
		if v2 != 0 || err2 == nil {
			t.Errorf("divide(10, 0) should return error")
		}
	})

	t.Run("checking error before using value", func(t *testing.T) {
		result := Value(42)

		if result.Error() != nil {
			t.Error("unexpected error")
		} else {
			v := result.Value()
			if v != 42 {
				t.Errorf("Value() = %d, want 42", v)
			}
		}
	})

	t.Run("transforming values with Map", func(t *testing.T) {
		parseNumber := func(s string) *Result[int] {
			var n int
			_, err := fmt.Sscanf(s, "%d", &n)
			return New(n, err)
		}

		result := parseNumber("42")
		doubled := Map(result, func(x int) int { return x * 2 })

		v, err := doubled.Get()
		if v != 84 || err != nil {
			t.Errorf("parsed and doubled = %d, %v; want 84, nil", v, err)
		}
	})
}

// BenchmarkResult benchmarks Result operations
func BenchmarkResult(b *testing.B) {
	b.Run("New", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = New(42, nil)
		}
	})

	b.Run("Value", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Value(42)
		}
	})

	b.Run("Error", func(b *testing.B) {
		err := errors.New("test error")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Error[int](err)
		}
	})

	b.Run("Get", func(b *testing.B) {
		result := Value(42)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = result.Get()
		}
	})

	b.Run("Map", func(b *testing.B) {
		result := Value(42)
		fn := func(x int) int { return x * 2 }
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Map(result, fn)
		}
	})

	b.Run("String", func(b *testing.B) {
		result := Value(42)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = result.String()
		}
	})
}
