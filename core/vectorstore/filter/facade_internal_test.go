package filter

import (
	"errors"
	"testing"
)

type recordingVisitor struct {
	visited Predicate
	err     error
	visits  int
}

func (v *recordingVisitor) Visit(predicate Predicate) error {
	v.visits++
	v.visited = predicate
	return v.err
}

var _ Visitor = (*recordingVisitor)(nil)

func TestOperatorVocabulary(t *testing.T) {
	tests := []struct {
		op     Operator
		name   string
		binary bool
		unary  bool
	}{
		{OpEqual, "EQ", true, false},
		{OpNotEqual, "NE", true, false},
		{OpLess, "LT", true, false},
		{OpLessEqual, "LE", true, false},
		{OpGreater, "GT", true, false},
		{OpGreaterEqual, "GE", true, false},
		{OpAnd, "AND", true, false},
		{OpOr, "OR", true, false},
		{OpNot, "NOT", false, true},
		{OpIn, "IN", true, false},
		{OpLike, "LIKE", true, false},
		{OpIs, "IS", true, false},
		{Operator("invalid"), "INVALID", false, false},
	}

	for _, tt := range tests {
		if got := tt.op.Name(); got != tt.name {
			t.Errorf("%q.Name() = %q, want %q", tt.op, got, tt.name)
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

func TestVisitorProcessesCompletePredicate(t *testing.T) {
	predicate := EQ("status", "active")
	visitor := &recordingVisitor{}
	if err := visitor.Visit(predicate); err != nil {
		t.Fatal(err)
	}
	if visitor.visited != predicate {
		t.Fatalf("visited = %T, want original predicate", visitor.visited)
	}
}

func TestVisitDispatchesInOrderAndStopsAtFirstError(t *testing.T) {
	predicate := EQ("status", "active")
	wantErr := errors.New("stop")
	first := &recordingVisitor{}
	second := &recordingVisitor{err: wantErr}
	third := &recordingVisitor{}

	if err := Visit(predicate, first, second, third); err != wantErr {
		t.Fatalf("Visit() error = %v, want %v", err, wantErr)
	}
	if first.visits != 1 || second.visits != 1 || third.visits != 0 {
		t.Fatalf("visitor calls = [%d %d %d], want [1 1 0]", first.visits, second.visits, third.visits)
	}
	if first.visited != predicate || second.visited != predicate {
		t.Fatal("Visit did not pass the original predicate to each visitor")
	}
}

func TestVisitRejectsInvalidInputsBeforeDispatch(t *testing.T) {
	visitor := &recordingVisitor{}
	if err := Visit(&BinaryExpr{}, visitor); err == nil {
		t.Fatal("Visit accepted an invalid predicate")
	}
	if visitor.visits != 0 {
		t.Fatal("Visit dispatched an invalid predicate")
	}

	predicate := EQ("status", "active")
	if err := Visit(predicate, visitor, nil); err == nil {
		t.Fatal("Visit accepted a nil visitor")
	}
	var nilVisitor *recordingVisitor
	if err := Visit(predicate, visitor, nilVisitor); err == nil {
		t.Fatal("Visit accepted a typed nil visitor")
	}
	if visitor.visits != 0 {
		t.Fatal("Visit dispatched before rejecting a nil visitor")
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
	if _, err := (*Literal)(nil).AsString(); err == nil {
		t.Fatal("AsString accepted a nil literal")
	}
	if _, err := (*Literal)(nil).AsNumber(); err == nil {
		t.Fatal("AsNumber accepted a nil literal")
	}
	if _, err := (*Literal)(nil).AsBool(); err == nil {
		t.Fatal("AsBool accepted a nil literal")
	}

	numberLiteral := NewLiteral(42)
	if got, err := numberLiteral.AsNumber(); err != nil || got.String() != "42" {
		t.Fatalf("AsNumber() = %v, %v", got, err)
	}
	precise := &Literal{Kind: LiteralNumber, Value: "9007199254740993"}
	if got, err := precise.AsNumber(); err != nil || got.String() != precise.Value {
		t.Fatalf("AsNumber() lost precision: %v, %v", got, err)
	}
	huge := &Literal{Kind: LiteralNumber, Value: "1e400"}
	if got, err := huge.AsNumber(); err != nil || got.String() != huge.Value {
		t.Fatalf("AsNumber() rejected exact large exponent: %v, %v", got, err)
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

	expressions := []Predicate{
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
	if binary.Start() != start || binary.End() != end || !binary.Equal(&BinaryExpr{Left: NewIdent("field"), Op: OpEqual, Right: NewLiteral("value")}) {
		t.Fatal("binary methods are inconsistent")
	}
	if (&BinaryExpr{Left: ident}).Start() != (Position{}) || (&BinaryExpr{Right: literal}).End() != (Position{}) {
		t.Fatal("programmatic binary positions must remain zero")
	}
	if (&BinaryExpr{}).Start() != (Position{}) || (&BinaryExpr{}).End() != (Position{}) || (*BinaryExpr)(nil).Start() != (Position{}) || (*BinaryExpr)(nil).End() != (Position{}) {
		t.Fatal("empty binary positions are inconsistent")
	}

	unary := &UnaryExpr{Op: OpNot, Right: binary, start: start, end: end}
	if unary.Start() != start || unary.End() != end || !unary.Equal(&UnaryExpr{Op: OpNot, Right: binary}) {
		t.Fatal("unary methods are inconsistent")
	}
	if (&UnaryExpr{Right: binary}).End() != (Position{}) || (&UnaryExpr{}).End() != (Position{}) || (*UnaryExpr)(nil).Start() != (Position{}) || (*UnaryExpr)(nil).End() != (Position{}) {
		t.Fatal("programmatic unary positions must remain zero")
	}

	indexed := &IndexExpr{Left: ident, Index: literal, start: start, end: end}
	if indexed.Start() != start || indexed.End() != end || !indexed.Equal(&IndexExpr{Left: NewIdent("field"), Index: NewLiteral("value")}) {
		t.Fatal("index methods are inconsistent")
	}
	if (&IndexExpr{Left: ident}).Start() != (Position{}) || (&IndexExpr{}).Start() != (Position{}) || (*IndexExpr)(nil).Start() != (Position{}) || (*IndexExpr)(nil).End() != (Position{}) {
		t.Fatal("programmatic index positions must remain zero")
	}

	if !equalExpr(nil, nil) || equalExpr(nil, ident) || !equalExpr((*Ident)(nil), (*Ident)(nil)) {
		t.Fatal("nil expression equality is inconsistent")
	}
}
