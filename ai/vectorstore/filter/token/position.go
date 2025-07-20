package token

import (
	"strconv"
)

// NoPosition represents an invalid or unknown position in the source code.
// It can be used as a sentinel value when position information is not available.
var NoPosition = Position{}

// Position represents a location in the source code with line and column information.
// Both line and column numbers are 1-indexed to match common editor conventions.
// This is essential for providing accurate error messages and debugging information.
type Position struct {
	Line   int // Line number in the source, starting at 1
	Column int // Column number in the current line, starting at 1
}

// NewPosition creates a new Position starting at the beginning of the source.
// This is typically used when initializing a lexer or parser to track
// the current position during tokenization.
func NewPosition() Position {
	return Position{
		Line:   1,
		Column: 1,
	}
}

// ResetColumn resets the column position to 1 while keeping the current line.
// This is typically called when encountering a newline character during lexing,
// as the cursor moves to the beginning of the next line.
func (p *Position) ResetColumn() {
	p.Column = 1
}

// ResetLine resets the line position to 1 while keeping the current column.
// This method is rarely used in practice, but provides symmetry with ResetColumn
// and may be useful for specific parsing scenarios.
func (p *Position) ResetLine() {
	p.Line = 1
}

// Reset resets both line and column positions to their initial values (1, 1).
// This is useful when reusing a Position instance for parsing a new source
// or when restarting the parsing process from the beginning.
func (p *Position) Reset() {
	p.Line = 1
	p.Column = 1
}

// String returns a human-readable string representation of the position.
// The format "L:C" is commonly used in error messages and debugging output.
// This makes it easy to locate issues in the source code.
func (p *Position) String() string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Column)
}
