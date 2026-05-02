package token

import "strconv"

// NoPosition is the zero-value sentinel callers use when position
// information is unavailable.
var NoPosition = Position{}

// Position locates a byte in the source — line and column are
// 1-indexed to match editor conventions and standard error-message
// formats.
type Position struct {
	// Line is the 1-indexed line number.
	Line int

	// Column is the 1-indexed column within the current line.
	Column int
}

// NewPosition returns the start-of-source position, (1, 1).
func NewPosition() Position {
	return Position{Line: 1, Column: 1}
}

// ResetColumn moves Column back to 1 while keeping Line — called by
// the lexer on every newline.
func (p *Position) ResetColumn() { p.Column = 1 }

// ResetLine moves Line back to 1 while keeping Column. Rarely used;
// kept for symmetry with [Position.ResetColumn].
func (p *Position) ResetLine() { p.Line = 1 }

// Reset returns the position to (1, 1) — typically called when reusing
// a Position across multiple parses.
func (p *Position) Reset() {
	p.Line = 1
	p.Column = 1
}

// String renders the position as "line:column", the format conventional
// in compiler error messages.
func (p Position) String() string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Column)
}
