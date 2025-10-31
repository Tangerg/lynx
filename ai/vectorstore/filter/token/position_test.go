package token

import (
	"testing"
)

// TestNewPosition tests the NewPosition constructor
func TestNewPosition(t *testing.T) {
	pos := NewPosition()

	if pos.Line != 1 {
		t.Errorf("NewPosition() Line = %v, want 1", pos.Line)
	}

	if pos.Column != 1 {
		t.Errorf("NewPosition() Column = %v, want 1", pos.Column)
	}
}

// TestNoPosition tests the NoPosition sentinel value
func TestNoPosition(t *testing.T) {
	if NoPosition.Line != 0 {
		t.Errorf("NoPosition Line = %v, want 0", NoPosition.Line)
	}

	if NoPosition.Column != 0 {
		t.Errorf("NoPosition Column = %v, want 0", NoPosition.Column)
	}
}

// TestPositionResetColumn tests the ResetColumn method
func TestPositionResetColumn(t *testing.T) {
	tests := []struct {
		name         string
		initialLine  int
		initialCol   int
		expectedLine int
		expectedCol  int
	}{
		{
			name:         "Reset column from arbitrary position",
			initialLine:  5,
			initialCol:   10,
			expectedLine: 5,
			expectedCol:  1,
		},
		{
			name:         "Reset column when already at 1",
			initialLine:  3,
			initialCol:   1,
			expectedLine: 3,
			expectedCol:  1,
		},
		{
			name:         "Reset column from large column number",
			initialLine:  1,
			initialCol:   1000,
			expectedLine: 1,
			expectedCol:  1,
		},
		{
			name:         "Reset column at line 1",
			initialLine:  1,
			initialCol:   50,
			expectedLine: 1,
			expectedCol:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := Position{
				Line:   tt.initialLine,
				Column: tt.initialCol,
			}

			pos.ResetColumn()

			if pos.Line != tt.expectedLine {
				t.Errorf("After ResetColumn() Line = %v, want %v", pos.Line, tt.expectedLine)
			}

			if pos.Column != tt.expectedCol {
				t.Errorf("After ResetColumn() Column = %v, want %v", pos.Column, tt.expectedCol)
			}
		})
	}
}

// TestPositionResetLine tests the ResetLine method
func TestPositionResetLine(t *testing.T) {
	tests := []struct {
		name         string
		initialLine  int
		initialCol   int
		expectedLine int
		expectedCol  int
	}{
		{
			name:         "Reset line from arbitrary position",
			initialLine:  10,
			initialCol:   5,
			expectedLine: 1,
			expectedCol:  5,
		},
		{
			name:         "Reset line when already at 1",
			initialLine:  1,
			initialCol:   20,
			expectedLine: 1,
			expectedCol:  20,
		},
		{
			name:         "Reset line from large line number",
			initialLine:  10000,
			initialCol:   1,
			expectedLine: 1,
			expectedCol:  1,
		},
		{
			name:         "Reset line at column 1",
			initialLine:  50,
			initialCol:   1,
			expectedLine: 1,
			expectedCol:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := Position{
				Line:   tt.initialLine,
				Column: tt.initialCol,
			}

			pos.ResetLine()

			if pos.Line != tt.expectedLine {
				t.Errorf("After ResetLine() Line = %v, want %v", pos.Line, tt.expectedLine)
			}

			if pos.Column != tt.expectedCol {
				t.Errorf("After ResetLine() Column = %v, want %v", pos.Column, tt.expectedCol)
			}
		})
	}
}

// TestPositionReset tests the Reset method
func TestPositionReset(t *testing.T) {
	tests := []struct {
		name        string
		initialLine int
		initialCol  int
	}{
		{
			name:        "Reset from arbitrary position",
			initialLine: 10,
			initialCol:  20,
		},
		{
			name:        "Reset when already at initial position",
			initialLine: 1,
			initialCol:  1,
		},
		{
			name:        "Reset from large line and column",
			initialLine: 10000,
			initialCol:  5000,
		},
		{
			name:        "Reset from zero position",
			initialLine: 0,
			initialCol:  0,
		},
		{
			name:        "Reset from negative position",
			initialLine: -1,
			initialCol:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := Position{
				Line:   tt.initialLine,
				Column: tt.initialCol,
			}

			pos.Reset()

			if pos.Line != 1 {
				t.Errorf("After Reset() Line = %v, want 1", pos.Line)
			}

			if pos.Column != 1 {
				t.Errorf("After Reset() Column = %v, want 1", pos.Column)
			}
		})
	}
}

// TestPositionString tests the String method
func TestPositionString(t *testing.T) {
	tests := []struct {
		name     string
		line     int
		column   int
		expected string
	}{
		{
			name:     "Initial position",
			line:     1,
			column:   1,
			expected: "1:1",
		},
		{
			name:     "Arbitrary position",
			line:     10,
			column:   25,
			expected: "10:25",
		},
		{
			name:     "Large line number",
			line:     1000,
			column:   1,
			expected: "1000:1",
		},
		{
			name:     "Large column number",
			line:     1,
			column:   500,
			expected: "1:500",
		},
		{
			name:     "Both large numbers",
			line:     9999,
			column:   9999,
			expected: "9999:9999",
		},
		{
			name:     "NoPosition equivalent",
			line:     0,
			column:   0,
			expected: "0:0",
		},
		{
			name:     "Single digit line and column",
			line:     5,
			column:   7,
			expected: "5:7",
		},
		{
			name:     "Mixed digit counts",
			line:     123,
			column:   4,
			expected: "123:4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := Position{
				Line:   tt.line,
				Column: tt.column,
			}

			result := pos.String()

			if result != tt.expected {
				t.Errorf("String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestPositionStringFormat tests the format of the String output
func TestPositionStringFormat(t *testing.T) {
	pos := Position{Line: 42, Column: 17}
	result := pos.String()

	// Check that it contains the colon separator
	if !containsChar(result, ':') {
		t.Errorf("String() output should contain ':' separator, got %v", result)
	}

	// Check that it starts with the line number
	if result[0] != '4' {
		t.Errorf("String() should start with line number, got %v", result)
	}
}

// TestPositionMutability tests that Position is properly mutable
func TestPositionMutability(t *testing.T) {
	pos := Position{Line: 1, Column: 1}

	// Test direct field modification
	pos.Line = 5
	pos.Column = 10

	if pos.Line != 5 || pos.Column != 10 {
		t.Errorf("Position should be mutable, got Line=%v, Column=%v", pos.Line, pos.Column)
	}

	// Test method modification
	pos.ResetColumn()
	if pos.Column != 1 {
		t.Errorf("ResetColumn() should modify column, got %v", pos.Column)
	}

	if pos.Line != 5 {
		t.Errorf("ResetColumn() should not modify line, got %v", pos.Line)
	}
}

// TestPositionCopy tests that Position can be copied properly
func TestPositionCopy(t *testing.T) {
	original := Position{Line: 10, Column: 20}

	// Value copy
	copy := original

	if copy.Line != original.Line || copy.Column != original.Column {
		t.Errorf("Position copy failed, got %v, want %v", copy, original)
	}

	// Modify copy and ensure original is unchanged
	copy.Line = 99
	copy.Column = 99

	if original.Line == 99 || original.Column == 99 {
		t.Errorf("Modifying copy should not affect original, original=%v", original)
	}
}

// TestPositionPointerMethods tests that pointer receiver methods work correctly
func TestPositionPointerMethods(t *testing.T) {
	pos := Position{Line: 5, Column: 10}
	ptr := &pos

	// Test that pointer methods modify the original
	ptr.ResetColumn()
	if pos.Column != 1 {
		t.Errorf("Pointer method should modify original, Column=%v", pos.Column)
	}

	ptr.ResetLine()
	if pos.Line != 1 {
		t.Errorf("Pointer method should modify original, Line=%v", pos.Line)
	}

	// Reset and test again
	pos.Line = 10
	pos.Column = 20
	ptr.Reset()

	if pos.Line != 1 || pos.Column != 1 {
		t.Errorf("Pointer Reset() should modify original, got %v", pos)
	}
}

// TestPositionValueReceiver tests that String method works with value receiver
func TestPositionValueReceiver(t *testing.T) {
	pos := Position{Line: 5, Column: 10}

	// Call String on value (not pointer)
	str1 := pos.String()
	expected := "5:10"

	if str1 != expected {
		t.Errorf("Value receiver String() = %v, want %v", str1, expected)
	}

	// Call String on pointer
	str2 := (&pos).String()

	if str2 != expected {
		t.Errorf("Pointer receiver String() = %v, want %v", str2, expected)
	}

	// Both should return the same result
	if str1 != str2 {
		t.Errorf("Value and pointer String() should match, got %v and %v", str1, str2)
	}
}

// TestPositionSequentialOperations tests a sequence of position operations
func TestPositionSequentialOperations(t *testing.T) {
	pos := NewPosition()

	// Simulate reading characters on first line
	pos.Column = 10
	if pos.String() != "1:10" {
		t.Errorf("After moving to column 10, got %v, want 1:10", pos.String())
	}

	// Simulate newline
	pos.Line++
	pos.ResetColumn()
	if pos.String() != "2:1" {
		t.Errorf("After newline, got %v, want 2:1", pos.String())
	}

	// Continue reading
	pos.Column = 15
	if pos.String() != "2:15" {
		t.Errorf("After moving to column 15, got %v, want 2:15", pos.String())
	}

	// Another newline
	pos.Line++
	pos.ResetColumn()
	if pos.String() != "3:1" {
		t.Errorf("After second newline, got %v, want 3:1", pos.String())
	}

	// Reset everything
	pos.Reset()
	if pos.String() != "1:1" {
		t.Errorf("After reset, got %v, want 1:1", pos.String())
	}
}

// TestPositionComparison tests comparing positions
func TestPositionComparison(t *testing.T) {
	pos1 := Position{Line: 1, Column: 1}
	pos2 := Position{Line: 1, Column: 1}
	pos3 := Position{Line: 2, Column: 1}

	// Test equality
	if pos1 != pos2 {
		t.Errorf("Positions with same values should be equal")
	}

	// Test inequality
	if pos1 == pos3 {
		t.Errorf("Positions with different values should not be equal")
	}

	// Test NoPosition
	if NoPosition == pos1 {
		t.Errorf("NoPosition should not equal valid position")
	}
}

// TestPositionEdgeCases tests edge cases and boundary conditions
func TestPositionEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		test func(*testing.T)
	}{
		{
			name: "Zero values",
			test: func(t *testing.T) {
				var pos Position
				if pos.Line != 0 || pos.Column != 0 {
					t.Errorf("Zero-value Position should have Line=0, Column=0")
				}
				if pos.String() != "0:0" {
					t.Errorf("Zero-value Position.String() = %v, want 0:0", pos.String())
				}
			},
		},
		{
			name: "Negative values",
			test: func(t *testing.T) {
				pos := Position{Line: -1, Column: -5}
				result := pos.String()
				expected := "-1:-5"
				if result != expected {
					t.Errorf("Negative position String() = %v, want %v", result, expected)
				}
			},
		},
		{
			name: "Very large values",
			test: func(t *testing.T) {
				pos := Position{Line: 999999999, Column: 888888888}
				result := pos.String()
				expected := "999999999:888888888"
				if result != expected {
					t.Errorf("Large position String() = %v, want %v", result, expected)
				}
			},
		},
		{
			name: "Multiple resets",
			test: func(t *testing.T) {
				pos := Position{Line: 100, Column: 200}
				pos.Reset()
				pos.Reset()
				pos.Reset()
				if pos.Line != 1 || pos.Column != 1 {
					t.Errorf("Multiple resets should keep position at 1:1")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// Helper function to check if a string contains a specific character
func containsChar(s string, c rune) bool {
	for _, ch := range s {
		if ch == c {
			return true
		}
	}
	return false
}

// BenchmarkNewPosition benchmarks the NewPosition constructor
func BenchmarkNewPosition(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewPosition()
	}
}

// BenchmarkPositionString benchmarks the String method
func BenchmarkPositionString(b *testing.B) {
	pos := Position{Line: 123, Column: 456}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pos.String()
	}
}

// BenchmarkPositionReset benchmarks the Reset method
func BenchmarkPositionReset(b *testing.B) {
	pos := Position{Line: 100, Column: 200}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pos.Reset()
	}
}

// BenchmarkPositionResetColumn benchmarks the ResetColumn method
func BenchmarkPositionResetColumn(b *testing.B) {
	pos := Position{Line: 100, Column: 200}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pos.ResetColumn()
	}
}

// TestPositionUsageScenario tests a realistic usage scenario
func TestPositionUsageScenario(t *testing.T) {
	// Simulate parsing a multi-line input
	input := "line 1\nline 2\nline 3"
	pos := NewPosition()

	for _, ch := range input {
		if ch == '\n' {
			pos.Line++
			pos.ResetColumn()
		} else {
			pos.Column++
		}
	}

	// After processing, position should be at line 3, column 7
	// (6 characters in "line 3" + 1 for 1-indexed)
	if pos.Line != 3 {
		t.Errorf("After parsing, Line = %v, want 3", pos.Line)
	}

	if pos.Column != 7 {
		t.Errorf("After parsing, Column = %v, want 7", pos.Column)
	}
}
