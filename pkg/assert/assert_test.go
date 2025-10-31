package assert

import (
	"errors"
	"testing"
)

func TestMust_Success(t *testing.T) {
	t.Run("string type", func(t *testing.T) {
		result := Must("hello", nil)
		if result != "hello" {
			t.Errorf("expected 'hello', got '%s'", result)
		}
	})

	t.Run("int type", func(t *testing.T) {
		result := Must(42, nil)
		if result != 42 {
			t.Errorf("expected 42, got %d", result)
		}
	})

	t.Run("struct type", func(t *testing.T) {
		type TestStruct struct {
			Name string
			Age  int
		}
		expected := TestStruct{Name: "Alice", Age: 30}
		result := Must(expected, nil)
		if result != expected {
			t.Errorf("expected %+v, got %+v", expected, result)
		}
	})

	t.Run("pointer type", func(t *testing.T) {
		value := 123
		ptr := &value
		result := Must(ptr, nil)
		if result != ptr {
			t.Errorf("expected pointer %p, got %p", ptr, result)
		}
		if *result != 123 {
			t.Errorf("expected dereferenced value 123, got %d", *result)
		}
	})

	t.Run("slice type", func(t *testing.T) {
		slice := []int{1, 2, 3}
		result := Must(slice, nil)
		if len(result) != 3 || result[0] != 1 {
			t.Errorf("expected slice [1 2 3], got %v", result)
		}
	})
}

func TestMust_Panic(t *testing.T) {
	t.Run("panic with error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic, but didn't panic")
			} else {
				err, ok := r.(error)
				if !ok {
					t.Errorf("expected error type in panic, got %T", r)
				}
				if err.Error() != "test error" {
					t.Errorf("expected 'test error', got '%s'", err.Error())
				}
			}
		}()

		Must("value", errors.New("test error"))
	})

	t.Run("panic with different error types", func(t *testing.T) {
		testCases := []struct {
			name string
			err  error
		}{
			{"simple error", errors.New("simple")},
			{"formatted error", errors.New("error: code 500")},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				defer func() {
					if r := recover(); r == nil {
						t.Error("expected panic")
					}
				}()
				Must(0, tc.err)
			})
		}
	})
}

func TestEnsure_Success(t *testing.T) {
	t.Run("true condition does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("unexpected panic: %v", r)
			}
		}()

		Ensure(true, "should not panic")
		Ensure(1 == 1, "math works")
		Ensure(len("hello") == 5, "length check")
	})
}

func TestEnsure_Panic(t *testing.T) {
	t.Run("false condition panics with message", func(t *testing.T) {
		expectedMessage := "condition failed"

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic, but didn't panic")
			} else {
				message, ok := r.(string)
				if !ok {
					t.Errorf("expected string in panic, got %T", r)
				}
				if message != expectedMessage {
					t.Errorf("expected '%s', got '%s'", expectedMessage, message)
				}
			}
		}()

		Ensure(false, expectedMessage)
	})

	t.Run("various false conditions", func(t *testing.T) {
		testCases := []struct {
			name      string
			condition bool
			message   string
		}{
			{"false literal", false, "literal false"},
			{"comparison", 1 > 2, "invalid comparison"},
			{"nil check", new(struct{}) == nil, "nil check failed"},
			{"empty string", len("") > 0, "string is empty"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				defer func() {
					if r := recover(); r == nil {
						t.Error("expected panic")
					} else if r.(string) != tc.message {
						t.Errorf("expected message '%s', got '%s'", tc.message, r.(string))
					}
				}()

				Ensure(tc.condition, tc.message)
			})
		}
	})
}

func TestMust_RealWorldScenarios(t *testing.T) {
	t.Run("file operation simulation", func(t *testing.T) {
		openFile := func(name string) (string, error) {
			if name == "" {
				return "", errors.New("empty filename")
			}
			return "file content", nil
		}

		result := Must(openFile("test.txt"))
		if result != "file content" {
			t.Errorf("expected 'file content', got '%s'", result)
		}
	})

	t.Run("configuration loading", func(t *testing.T) {
		type Config struct {
			Host string
			Port int
		}

		loadConfig := func() (Config, error) {
			return Config{Host: "localhost", Port: 8080}, nil
		}

		config := Must(loadConfig())
		if config.Host != "localhost" || config.Port != 8080 {
			t.Errorf("unexpected config: %+v", config)
		}
	})
}

func TestEnsure_RealWorldScenarios(t *testing.T) {
	t.Run("invariant checking", func(t *testing.T) {
		balance := 100
		withdrawAmount := 50

		Ensure(balance >= withdrawAmount, "insufficient funds")

		balance -= withdrawAmount
		if balance != 50 {
			t.Errorf("expected balance 50, got %d", balance)
		}
	})

	t.Run("precondition validation", func(t *testing.T) {
		processData := func(data []int) int {
			Ensure(len(data) > 0, "data cannot be empty")
			Ensure(data[0] >= 0, "first element must be non-negative")
			return data[0]
		}

		result := processData([]int{10, 20, 30})
		if result != 10 {
			t.Errorf("expected 10, got %d", result)
		}
	})
}

// Benchmark tests
func BenchmarkMust_Success(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Must(42, nil)
	}
}

func BenchmarkEnsure_True(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Ensure(true, "benchmark")
	}
}
