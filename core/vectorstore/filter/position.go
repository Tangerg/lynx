package filter

import "strconv"

// Position identifies a source line and column. Parsed expressions carry
// positions; programmatically constructed expressions use the zero value.
type Position struct {
	Line   int
	Column int
}

func (p Position) String() string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Column)
}
