package filter

import (
	"reflect"
	"testing"

	internalparser "github.com/Tangerg/lynx/core/vectorstore/filter/internal/parser"
	"github.com/Tangerg/lynx/core/vectorstore/filter/internal/token"
)

func TestOperatorVocabulary(t *testing.T) {
	tests := []struct {
		op         Operator
		name       string
		precedence int
		binary     bool
		unary      bool
	}{
		{OpEqual, "EQ", precedenceComparison, true, false},
		{OpNotEqual, "NE", precedenceComparison, true, false},
		{OpLess, "LT", precedenceComparison, true, false},
		{OpLessEqual, "LE", precedenceComparison, true, false},
		{OpGreater, "GT", precedenceComparison, true, false},
		{OpGreaterEqual, "GE", precedenceComparison, true, false},
		{OpAnd, "AND", precedenceAnd, true, false},
		{OpOr, "OR", precedenceOr, true, false},
		{OpNot, "NOT", precedenceNot, false, true},
		{OpIn, "IN", precedenceMatch, true, false},
		{OpLike, "LIKE", precedenceMatch, true, false},
		{OpIs, "IS", precedenceComparison, true, false},
		{Operator("invalid"), "INVALID", precedenceLowest, false, false},
	}

	for _, tt := range tests {
		if got := tt.op.Name(); got != tt.name {
			t.Errorf("%q.Name() = %q, want %q", tt.op, got, tt.name)
		}
		if got := tt.op.Precedence(); got != tt.precedence {
			t.Errorf("%q.Precedence() = %d, want %d", tt.op, got, tt.precedence)
		}
		if got := tt.op.IsBinaryOperator(); got != tt.binary {
			t.Errorf("%q.IsBinaryOperator() = %t, want %t", tt.op, got, tt.binary)
		}
		if got := tt.op.IsUnaryOperator(); got != tt.unary {
			t.Errorf("%q.IsUnaryOperator() = %t, want %t", tt.op, got, tt.unary)
		}
		if tt.op.String() != string(tt.op) || !tt.op.Is(tt.op) {
			t.Errorf("%q string/identity helpers are inconsistent", tt.op)
		}
	}

	if !OpEqual.IsEqualityOperator() || !OpLess.IsOrderingOperator() ||
		!OpGreaterEqual.IsComparisonOperator() || !OpAnd.IsLogicalOperator() ||
		!OpIn.IsMatchingOperator() || !OpIs.IsNullOperator() {
		t.Fatal("operator category helper rejected a member")
	}
	if OpLike.IsEqualityOperator() || OpOr.IsOrderingOperator() ||
		OpNot.IsComparisonOperator() || OpEqual.IsLogicalOperator() ||
		OpAnd.IsMatchingOperator() || OpEqual.IsNullOperator() {
		t.Fatal("operator category helper accepted a non-member")
	}
}

func TestLiteralVocabularyAndConstructors(t *testing.T) {
	stringLiteral := NewLiteral("lynx")
	if !stringLiteral.IsString() || stringLiteral.IsNumber() || stringLiteral.IsBool() || stringLiteral.IsNull() {
		t.Fatal("string literal kind helpers are inconsistent")
	}
	if got, err := stringLiteral.AsString(); err != nil || got != "lynx" {
		t.Fatalf("AsString() = %q, %v", got, err)
	}
	if _, err := stringLiteral.AsNumber(); err == nil {
		t.Fatal("AsNumber accepted a string")
	}
	if _, err := stringLiteral.AsBool(); err == nil {
		t.Fatal("AsBool accepted a string")
	}

	numberLiteral := NewLiteral(42)
	if got, err := numberLiteral.AsNumber(); err != nil || got != 42 {
		t.Fatalf("AsNumber() = %v, %v", got, err)
	}
	if _, err := (&Literal{Kind: LiteralNumber, Value: "not-a-number"}).AsNumber(); err == nil {
		t.Fatal("AsNumber accepted an invalid number")
	}

	boolLiteral := NewLiteral(true)
	if !boolLiteral.IsBool() {
		t.Fatal("bool literal not recognized")
	}
	if got, err := boolLiteral.AsBool(); err != nil || !got {
		t.Fatalf("AsBool() = %t, %v", got, err)
	}
	if _, err := (&Literal{Kind: LiteralBool, Value: "not-a-bool"}).AsBool(); err == nil {
		t.Fatal("AsBool accepted an invalid boolean")
	}

	nullLiteral := &Literal{Kind: LiteralNull, Value: "null"}
	if !nullLiteral.IsNull() || nullLiteral.IsSameKind(stringLiteral) || !stringLiteral.IsSameKind(NewLiteral("other")) {
		t.Fatal("literal kind comparison is inconsistent")
	}
	if (*Literal)(nil).IsSameKind(stringLiteral) || stringLiteral.IsSameKind(nil) {
		t.Fatal("nil literal kinds must not match")
	}

	if got := NewLiteral(stringLiteral); got != stringLiteral {
		t.Fatal("NewLiteral did not preserve an existing literal")
	}
	if got := NewLiterals([]string{"a", "b"}); len(got) != 2 || got[1].Value != "b" {
		t.Fatalf("NewLiterals() = %#v", got)
	}
	if _, err := newLiteral(nil); err == nil {
		t.Fatal("newLiteral accepted nil")
	}
	if _, err := newLiteral(struct{}{}); err == nil {
		t.Fatal("newLiteral accepted a struct")
	}

	listInputs := []any{
		[]int{1}, []int8{1}, []int16{1}, []int32{1}, []int64{1},
		[]uint{1}, []uint8{1}, []uint16{1}, []uint32{1}, []uint64{1},
		[]float32{1}, []float64{1}, []string{"a"}, []bool{true},
		[]*Literal{stringLiteral},
	}
	for _, input := range listInputs {
		list, err := newListLiteral(input)
		if err != nil || len(list.Values) != 1 {
			t.Fatalf("newListLiteral(%T) = %#v, %v", input, list, err)
		}
	}
	existingList := &ListLiteral{Values: []*Literal{stringLiteral}}
	if got, err := newListLiteral(existingList); err != nil || got != existingList {
		t.Fatalf("newListLiteral(existing) = %#v, %v", got, err)
	}
	if _, err := newListLiteral(struct{}{}); err == nil {
		t.Fatal("newListLiteral accepted a struct")
	}
	if got := NewListLiteral([]int{1, 2}); len(got.Values) != 2 {
		t.Fatalf("NewListLiteral() = %#v", got)
	}
}

func TestSemanticConstructorsCoverVocabulary(t *testing.T) {
	ident := NewIdent("field")
	if NewIdent(ident) != ident {
		t.Fatal("NewIdent did not preserve an existing identifier")
	}
	if _, err := newIdent(42); err == nil {
		t.Fatal("newIdent accepted a number")
	}

	index := Index("metadata", "author")
	nested := Index(index, 0)
	fromIdent := Index(ident, NewLiteral("key"))
	if index.Left == nil || nested.Left != index || fromIdent.Left != ident {
		t.Fatal("Index did not preserve its left operand")
	}

	expressions := []Expr{
		EQ("name", "lynx"), NE("name", "other"),
		LT("rank", 10), LE("rank", 10), GT("rank", 1), GE("rank", 1),
		In("tag", []string{"go", "ai"}), Like("name", "ly%"),
		IsNull("deleted_at"), IsNotNull("created_at"),
		And(EQ("a", 1), EQ("b", 2)), Or(EQ("a", 1), EQ("b", 2)),
		Not(EQ("disabled", true)), EQ(nested, "lynx"),
	}
	for _, expr := range expressions {
		if err := Validate(expr); err != nil {
			t.Fatalf("Validate(%T) = %v", expr, err)
		}
	}
}

func TestExprBuilderSuccessAndErrors(t *testing.T) {
	builder := NewExprBuilder().
		EQ("eq", 1).
		NE("ne", 2).
		LT("lt", 3).
		LE("le", 4).
		GT("gt", 5).
		GE("ge", 6).
		Like("title", "go%").
		In("tags", []string{"go", "ai"}).
		And(func(sub *ExprBuilder) { sub.EQ("nested_and", true) }).
		Or(func(sub *ExprBuilder) { sub.EQ("nested_or", true) }).
		Not(func(sub *ExprBuilder) { sub.EQ("nested_not", true) })
	expr, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(expr); err != nil {
		t.Fatal(err)
	}

	empty := NewExprBuilder()
	empty.and(nil)
	empty.or(nil)
	empty.Or(func(*ExprBuilder) {})
	empty.Not(func(*ExprBuilder) {})
	if expr, err := empty.Build(); err != nil || expr != nil {
		t.Fatalf("empty Build() = %#v, %v", expr, err)
	}

	badRight := NewExprBuilder().EQ("field", struct{}{}).NE("ignored", 1)
	badRight.In("ignored", []int{1}).And(func(*ExprBuilder) {})
	if _, err := badRight.Build(); err == nil {
		t.Fatal("builder accepted an invalid right operand")
	}
	if _, err := NewExprBuilder().EQ(struct{}{}, 1).Build(); err == nil {
		t.Fatal("builder accepted an invalid left operand")
	}
	if _, err := NewExprBuilder().In("field", struct{}{}).Build(); err == nil {
		t.Fatal("builder accepted an invalid list")
	}
	if _, err := NewExprBuilder().In(struct{}{}, []int{1}).Build(); err == nil {
		t.Fatal("builder accepted an invalid IN left operand")
	}
	if _, err := NewExprBuilder().And(func(sub *ExprBuilder) {
		sub.EQ("field", struct{}{})
	}).Build(); err == nil {
		t.Fatal("builder did not propagate a nested error")
	}
}

func TestSemanticNodeMethods(t *testing.T) {
	start, end := Position{Line: 2, Column: 3}, Position{Line: 2, Column: 8}
	if start.String() != "2:3" {
		t.Fatalf("Position.String() = %q", start.String())
	}

	ident := &Ident{Value: "field", start: start, end: end}
	if ident.Start() != start || ident.End() != end || !ident.Equal(&Ident{Value: "field"}) || ident.Equal(NewLiteral("field")) {
		t.Fatal("identifier methods are inconsistent")
	}
	if (*Ident)(nil).Start() != (Position{}) || (*Ident)(nil).End() != (Position{}) || (*Ident)(nil).Equal((*Ident)(nil)) {
		t.Fatal("nil identifier methods are inconsistent")
	}

	literal := &Literal{Kind: LiteralString, Value: "value", start: start, end: end}
	if literal.Start() != start || literal.End() != end || !literal.Equal(NewLiteral("value")) || literal.Equal(NewLiteral("other")) {
		t.Fatal("literal methods are inconsistent")
	}
	if (*Literal)(nil).Start() != (Position{}) || (*Literal)(nil).End() != (Position{}) || (*Literal)(nil).Equal((*Literal)(nil)) {
		t.Fatal("nil literal methods are inconsistent")
	}

	list := &ListLiteral{Values: []*Literal{literal}, start: start, end: end}
	if list.Start() != start || list.End() != end || !list.Equal(&ListLiteral{Values: []*Literal{NewLiteral("value")}}) {
		t.Fatal("list methods are inconsistent")
	}
	if list.Equal(&ListLiteral{}) || list.Equal(&ListLiteral{Values: []*Literal{NewLiteral("other")}}) || list.Equal(literal) {
		t.Fatal("list equality accepted a mismatch")
	}
	if (*ListLiteral)(nil).Start() != (Position{}) || (*ListLiteral)(nil).End() != (Position{}) || (*ListLiteral)(nil).Equal((*ListLiteral)(nil)) {
		t.Fatal("nil list methods are inconsistent")
	}

	binary := &BinaryExpr{Left: ident, Op: OpEqual, Right: literal, start: start, end: end}
	if binary.Start() != start || binary.End() != end || binary.Precedence() != precedenceComparison || !binary.Equal(&BinaryExpr{Left: NewIdent("field"), Op: OpEqual, Right: NewLiteral("value")}) {
		t.Fatal("binary methods are inconsistent")
	}
	if (&BinaryExpr{Left: ident}).Start() != start || (&BinaryExpr{Right: literal}).End() != end {
		t.Fatal("binary position fallback is inconsistent")
	}
	if (&BinaryExpr{}).Start() != (Position{}) || (&BinaryExpr{}).End() != (Position{}) || (*BinaryExpr)(nil).Start() != (Position{}) || (*BinaryExpr)(nil).End() != (Position{}) {
		t.Fatal("empty binary positions are inconsistent")
	}

	unary := &UnaryExpr{Op: OpNot, Right: binary, start: start, end: end}
	if unary.Start() != start || unary.End() != end || unary.Precedence() != precedenceNot || !unary.Equal(&UnaryExpr{Op: OpNot, Right: binary}) {
		t.Fatal("unary methods are inconsistent")
	}
	if (&UnaryExpr{Right: binary}).End() != end || (&UnaryExpr{}).End() != (Position{}) || (*UnaryExpr)(nil).Start() != (Position{}) || (*UnaryExpr)(nil).End() != (Position{}) {
		t.Fatal("unary position fallback is inconsistent")
	}

	indexed := &IndexExpr{Left: ident, Index: literal, start: start, end: end}
	if indexed.Start() != start || indexed.End() != end || !indexed.Equal(&IndexExpr{Left: NewIdent("field"), Index: NewLiteral("value")}) {
		t.Fatal("index methods are inconsistent")
	}
	if (&IndexExpr{Left: ident}).Start() != start || (&IndexExpr{}).Start() != (Position{}) || (*IndexExpr)(nil).Start() != (Position{}) || (*IndexExpr)(nil).End() != (Position{}) {
		t.Fatal("index position fallback is inconsistent")
	}

	if !equalExpr(nil, nil) || equalExpr(nil, ident) || !equalExpr((*Ident)(nil), (*Ident)(nil)) {
		t.Fatal("nil expression equality is inconsistent")
	}
}

func TestSemanticInternalConversion(t *testing.T) {
	operatorTokens := []token.Kind{
		token.EQ, token.NE, token.LT, token.LE, token.GT, token.GE,
		token.AND, token.OR, token.NOT, token.IN, token.LIKE, token.IS,
	}
	for _, kind := range operatorTokens {
		op, err := operatorFromToken(kind)
		if err != nil {
			t.Fatal(err)
		}
		got, err := tokenFromOperator(op)
		if err != nil || got != kind {
			t.Fatalf("operator round trip %v -> %q -> %v, %v", kind, op, got, err)
		}
	}
	if _, err := operatorFromToken(token.IDENT); err == nil {
		t.Fatal("operatorFromToken accepted IDENT")
	}
	if _, err := tokenFromOperator(Operator("invalid")); err == nil {
		t.Fatal("tokenFromOperator accepted an invalid operator")
	}

	literalKinds := []struct {
		token token.Kind
		kind  LiteralKind
	}{
		{token.STRING, LiteralString}, {token.NUMBER, LiteralNumber},
		{token.TRUE, LiteralBool}, {token.FALSE, LiteralBool}, {token.NULL, LiteralNull},
	}
	for _, tt := range literalKinds {
		if got, err := literalKindFromToken(tt.token); err != nil || got != tt.kind {
			t.Fatalf("literalKindFromToken(%v) = %q, %v", tt.token, got, err)
		}
	}
	if _, err := literalKindFromToken(token.IDENT); err == nil {
		t.Fatal("literalKindFromToken accepted IDENT")
	}

	for _, literal := range []*Literal{
		NewLiteral("value"), NewLiteral(1), NewLiteral(true), NewLiteral(false),
		{Kind: LiteralNull, Value: "null"},
	} {
		if _, err := literalToken(literal); err != nil {
			t.Fatalf("literalToken(%#v) = %v", literal, err)
		}
	}
	if _, err := literalToken(&Literal{Kind: LiteralBool, Value: "invalid"}); err == nil {
		t.Fatal("literalToken accepted an invalid boolean")
	}
	if _, err := literalToken(&Literal{Kind: LiteralKind("invalid")}); err == nil {
		t.Fatal("literalToken accepted an invalid kind")
	}

	inputs := []string{
		`a == 1`, `a != 1`, `a < 1`, `a <= 1`, `a > 1`, `a >= 1`,
		`a == 1 and b == 2`, `a == 1 or b == 2`, `not (a == 1)`,
		`tags in ('a', 'b')`, `name like 'a%'`, `deleted is null`,
		`metadata['author'] == 'lynx'`,
	}
	for _, input := range inputs {
		internal, err := internalparser.Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		public, err := fromInternal(internal)
		if err != nil {
			t.Fatalf("fromInternal(%q) = %v", input, err)
		}
		roundTrip, err := toInternal(public)
		if err != nil || !internal.Equal(roundTrip) {
			t.Fatalf("conversion round trip for %q failed: %v", input, err)
		}
	}
	if got, err := fromInternal(nil); err != nil || got != nil {
		t.Fatalf("fromInternal(nil) = %#v, %v", got, err)
	}
	if got, err := toInternal(nil); err != nil || got != nil {
		t.Fatalf("toInternal(nil) = %#v, %v", got, err)
	}

	invalid := []Expr{
		&Literal{Kind: LiteralKind("invalid")},
		&UnaryExpr{Op: Operator("invalid"), Right: EQ("a", 1)},
		&BinaryExpr{Left: NewIdent("a"), Op: Operator("invalid"), Right: NewLiteral(1)},
		&IndexExpr{Left: NewIdent("a"), Index: &Literal{Kind: LiteralKind("invalid")}},
	}
	for _, expr := range invalid {
		if _, err := toInternal(expr); err == nil {
			t.Fatalf("toInternal accepted invalid %T", expr)
		}
		if got := simplify(expr); !reflect.DeepEqual(got, expr) {
			t.Fatalf("simplify changed invalid %T", expr)
		}
	}
}
