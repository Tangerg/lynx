package filter

// Index builds `left[index]`. Left can be a name, an existing
// identifier, or a previously built index expression — the latter
// supports nested access like `matrix[1][2]`. Index must be numeric or
// a string.
func Index[L IdentifierValue | *IndexExpr, I Number | string | *Literal](left L, index I) *IndexExpr {
	return &IndexExpr{Left: leftOperand(left), Index: NewLiteral(index)}
}
