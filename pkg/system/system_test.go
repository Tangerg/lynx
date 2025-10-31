package system

import (
	"runtime"
	"testing"
)

// TestLineSeparator tests the LineSeparator function
func TestLineSeparator(t *testing.T) {
	t.Run("returns non-empty string", func(t *testing.T) {
		separator := LineSeparator()
		if separator == "" {
			t.Error("LineSeparator() returned empty string")
		}
	})

	t.Run("returns correct separator for current OS", func(t *testing.T) {
		separator := LineSeparator()

		switch runtime.GOOS {
		case "windows":
			expected := "\r\n"
			if separator != expected {
				t.Errorf("LineSeparator() = %q, want %q for Windows", separator, expected)
			}
		default:
			expected := "\n"
			if separator != expected {
				t.Errorf("LineSeparator() = %q, want %q for Unix-like systems", separator, expected)
			}
		}
	})

	t.Run("returns consistent value", func(t *testing.T) {
		first := LineSeparator()
		second := LineSeparator()

		if first != second {
			t.Errorf("LineSeparator() returned inconsistent values: %q vs %q", first, second)
		}
	})

	t.Run("returns same instance", func(t *testing.T) {
		// Multiple calls should return the same string value
		results := make([]string, 100)
		for i := 0; i < 100; i++ {
			results[i] = LineSeparator()
		}

		first := results[0]
		for i, result := range results {
			if result != first {
				t.Errorf("call %d returned %q, want %q", i, result, first)
			}
		}
	})
}

// TestLineSeparator_WindowsSpecific tests Windows-specific behavior
func TestLineSeparator_WindowsSpecific(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test")
	}

	t.Run("returns CRLF on Windows", func(t *testing.T) {
		separator := LineSeparator()
		expected := "\r\n"

		if separator != expected {
			t.Errorf("LineSeparator() = %q, want %q", separator, expected)
		}
	})

	t.Run("has correct length on Windows", func(t *testing.T) {
		separator := LineSeparator()
		expectedLen := 2

		if len(separator) != expectedLen {
			t.Errorf("len(LineSeparator()) = %d, want %d", len(separator), expectedLen)
		}
	})

	t.Run("contains carriage return and line feed on Windows", func(t *testing.T) {
		separator := LineSeparator()

		if len(separator) != 2 {
			t.Fatalf("separator length = %d, want 2", len(separator))
		}

		if separator[0] != '\r' {
			t.Errorf("first character = %q, want '\\r'", separator[0])
		}

		if separator[1] != '\n' {
			t.Errorf("second character = %q, want '\\n'", separator[1])
		}
	})
}

// TestLineSeparator_UnixSpecific tests Unix-specific behavior
func TestLineSeparator_UnixSpecific(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific test")
	}

	t.Run("returns LF on Unix-like systems", func(t *testing.T) {
		separator := LineSeparator()
		expected := "\n"

		if separator != expected {
			t.Errorf("LineSeparator() = %q, want %q", separator, expected)
		}
	})

	t.Run("has correct length on Unix-like systems", func(t *testing.T) {
		separator := LineSeparator()
		expectedLen := 1

		if len(separator) != expectedLen {
			t.Errorf("len(LineSeparator()) = %d, want %d", len(separator), expectedLen)
		}
	})

	t.Run("contains only line feed on Unix-like systems", func(t *testing.T) {
		separator := LineSeparator()

		if len(separator) != 1 {
			t.Fatalf("separator length = %d, want 1", len(separator))
		}

		if separator[0] != '\n' {
			t.Errorf("character = %q, want '\\n'", separator[0])
		}
	})

	t.Run("works on different Unix variants", func(t *testing.T) {
		unixLikeSystems := []string{"linux", "darwin", "freebsd", "openbsd", "netbsd", "dragonfly", "solaris", "aix"}

		isUnixLike := false
		for _, sys := range unixLikeSystems {
			if runtime.GOOS == sys {
				isUnixLike = true
				break
			}
		}

		if !isUnixLike {
			t.Skipf("Unknown OS: %s, assuming Unix-like behavior", runtime.GOOS)
		}

		separator := LineSeparator()
		expected := "\n"

		if separator != expected {
			t.Errorf("LineSeparator() = %q, want %q for %s", separator, expected, runtime.GOOS)
		}
	})
}

// TestLineSeparator_ByteRepresentation tests byte representation
func TestLineSeparator_ByteRepresentation(t *testing.T) {
	t.Run("has correct byte representation", func(t *testing.T) {
		separator := LineSeparator()
		bytes := []byte(separator)

		switch runtime.GOOS {
		case "windows":
			expectedBytes := []byte{'\r', '\n'}
			if len(bytes) != len(expectedBytes) {
				t.Fatalf("byte length = %d, want %d", len(bytes), len(expectedBytes))
			}
			for i, b := range bytes {
				if b != expectedBytes[i] {
					t.Errorf("byte[%d] = %d, want %d", i, b, expectedBytes[i])
				}
			}
		default:
			expectedBytes := []byte{'\n'}
			if len(bytes) != len(expectedBytes) {
				t.Fatalf("byte length = %d, want %d", len(bytes), len(expectedBytes))
			}
			for i, b := range bytes {
				if b != expectedBytes[i] {
					t.Errorf("byte[%d] = %d, want %d", i, b, expectedBytes[i])
				}
			}
		}
	})

	t.Run("can be used as byte slice", func(t *testing.T) {
		separator := LineSeparator()
		bytes := []byte(separator)

		if len(bytes) == 0 {
			t.Error("byte slice is empty")
		}

		// Verify it's a valid byte slice
		reconstructed := string(bytes)
		if reconstructed != separator {
			t.Errorf("reconstructed string = %q, want %q", reconstructed, separator)
		}
	})
}

// TestLineSeparator_StringOperations tests string operations
func TestLineSeparator_StringOperations(t *testing.T) {
	t.Run("can concatenate with other strings", func(t *testing.T) {
		separator := LineSeparator()
		line1 := "Hello"
		line2 := "World"

		combined := line1 + separator + line2

		if combined == "" {
			t.Error("concatenation resulted in empty string")
		}

		expectedPrefix := "Hello"
		if len(combined) < len(expectedPrefix) {
			t.Fatalf("combined string too short: %q", combined)
		}

		if combined[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("combined string doesn't start with %q", expectedPrefix)
		}
	})

	t.Run("can be used in string builder", func(t *testing.T) {
		separator := LineSeparator()
		var builder []string

		builder = append(builder, "Line 1")
		builder = append(builder, separator)
		builder = append(builder, "Line 2")
		builder = append(builder, separator)
		builder = append(builder, "Line 3")

		result := ""
		for _, s := range builder {
			result += s
		}

		if result == "" {
			t.Error("built string is empty")
		}

		// Should contain the separator
		expectedCount := 2
		actualCount := 0
		for i := 0; i < len(result); {
			if i+len(separator) <= len(result) && result[i:i+len(separator)] == separator {
				actualCount++
				i += len(separator)
			} else {
				i++
			}
		}

		if actualCount != expectedCount {
			t.Errorf("separator count = %d, want %d", actualCount, expectedCount)
		}
	})

	t.Run("can be compared with string literals", func(t *testing.T) {
		separator := LineSeparator()

		switch runtime.GOOS {
		case "windows":
			if separator == "\n" {
				t.Error("Windows separator should not equal Unix separator")
			}
			if separator != "\r\n" {
				t.Error("Windows separator should equal CRLF")
			}
		default:
			if separator == "\r\n" {
				t.Error("Unix separator should not equal Windows separator")
			}
			if separator != "\n" {
				t.Error("Unix separator should equal LF")
			}
		}
	})
}

// TestLineSeparator_EdgeCases tests edge cases
func TestLineSeparator_EdgeCases(t *testing.T) {
	t.Run("not nil", func(t *testing.T) {
		separator := LineSeparator()
		// String cannot be nil in Go, but we check it's not empty
		if separator == "" {
			t.Error("separator should not be empty")
		}
	})

	t.Run("is valid UTF-8", func(t *testing.T) {
		separator := LineSeparator()

		// Check if it's valid UTF-8
		for i, r := range separator {
			if r == '\ufffd' && separator[i] != 0xEF { // Unicode replacement character
				t.Error("separator contains invalid UTF-8")
			}
		}
	})

	t.Run("has printable escape sequence", func(t *testing.T) {
		separator := LineSeparator()

		// Verify the separator contains only control characters
		for _, r := range separator {
			if r != '\r' && r != '\n' {
				t.Errorf("unexpected character in separator: %q", r)
			}
		}
	})
}

// TestLineSeparator_PackageVariable tests the package-level variable
func TestLineSeparator_PackageVariable(t *testing.T) {
	t.Run("package variable is initialized", func(t *testing.T) {
		// The package variable should be initialized by init()
		if lineSeparator == "" {
			t.Error("lineSeparator package variable is empty")
		}
	})

	t.Run("function returns package variable", func(t *testing.T) {
		// Function should return the package variable value
		result := LineSeparator()
		if result != lineSeparator {
			t.Errorf("LineSeparator() = %q, want %q", result, lineSeparator)
		}
	})

	t.Run("package variable matches OS", func(t *testing.T) {
		switch runtime.GOOS {
		case "windows":
			if lineSeparator != "\r\n" {
				t.Errorf("lineSeparator = %q, want \\r\\n for Windows", lineSeparator)
			}
		default:
			if lineSeparator != "\n" {
				t.Errorf("lineSeparator = %q, want \\n for Unix-like systems", lineSeparator)
			}
		}
	})
}

// TestLineSeparator_Concurrent tests concurrent access
func TestLineSeparator_Concurrent(t *testing.T) {
	t.Run("thread-safe access", func(t *testing.T) {
		const goroutines = 100
		const callsPerGoroutine = 100

		results := make(chan string, goroutines*callsPerGoroutine)
		done := make(chan bool, goroutines)

		for i := 0; i < goroutines; i++ {
			go func() {
				for j := 0; j < callsPerGoroutine; j++ {
					results <- LineSeparator()
				}
				done <- true
			}()
		}

		// Wait for all goroutines to finish
		for i := 0; i < goroutines; i++ {
			<-done
		}
		close(results)

		// Verify all results are the same
		first := <-results
		count := 1
		for result := range results {
			count++
			if result != first {
				t.Errorf("concurrent call returned %q, want %q", result, first)
			}
		}

		expectedCount := goroutines * callsPerGoroutine
		if count != expectedCount {
			t.Errorf("received %d results, want %d", count, expectedCount)
		}
	})
}

// BenchmarkLineSeparator benchmarks the LineSeparator function
func BenchmarkLineSeparator(b *testing.B) {
	b.Run("single call", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = LineSeparator()
		}
	})

	b.Run("with string concatenation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = "Line 1" + LineSeparator() + "Line 2"
		}
	})

	b.Run("parallel calls", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = LineSeparator()
			}
		})
	})
}

// ExampleLineSeparator demonstrates basic usage
func ExampleLineSeparator() {
	// Get the system line separator
	separator := LineSeparator()

	// Use it to join lines
	text := "Line 1" + separator + "Line 2" + separator + "Line 3"

	_ = text
	// Output will vary by OS:
	// On Unix: "Line 1\nLine 2\nLine 3"
	// On Windows: "Line 1\r\nLine 2\r\nLine 3"
}

// ExampleLineSeparator_multiline demonstrates multi-line text construction
func ExampleLineSeparator_multiline() {
	lines := []string{"First line", "Second line", "Third line"}
	separator := LineSeparator()

	result := ""
	for i, line := range lines {
		result += line
		if i < len(lines)-1 {
			result += separator
		}
	}

	_ = result
}
