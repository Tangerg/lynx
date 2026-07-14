package token

import "strconv"

// NoPosition is the zero-value sentinel callers use when position
// information is unavailable.
var NoPosition = Position{}

// Position locates a byte in the source — line and column are
// 1-indexed to match editor conventions and standard error-message
// formats.
type Position struct {
	Line   int
	Column int
}

func NewPosition() Position {
	return Position{Line: 1, Column: 1}
}

func (p *Position) ResetColumn() { p.Column = 1 }

func (p *Position) Reset() {
	p.Line = 1
	p.Column = 1
}

func (p Position) String() string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Column)
}
