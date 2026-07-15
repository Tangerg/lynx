package filtercompile_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filtercompile"
)

func TestDispatchRejectsUnsupportedOperators(t *testing.T) {
	nullTest := mustParseBinary(t, `field is null`)
	if err := filtercompile.DispatchBinary(nullTest, nil, nil, nil, nil); err == nil {
		t.Fatal("DispatchBinary accepted a null operator")
	}

	invalidUnary := &filter.UnaryExpr{Op: filter.OpAnd, Right: filter.EQ("field", 1)}
	if err := filtercompile.DispatchUnary(invalidUnary, nil); err == nil {
		t.Fatal("DispatchUnary accepted a binary operator")
	}
}

func TestLiteralAsKey(t *testing.T) {
	tests := []struct {
		name    string
		literal *filter.Literal
		want    string
		wantErr bool
	}{
		{name: "string", literal: filter.NewLiteral("name"), want: "name"},
		{name: "signed integer", literal: filter.NewLiteral(42), want: "42"},
		{name: "unsigned integer", literal: filter.NewLiteral(uint64(math.MaxInt64)), want: "9223372036854775807"},
		{name: "integral decimal", literal: &filter.Literal{Kind: filter.LiteralNumber, Value: "4.0"}, want: "4"},
		{name: "negative", literal: filter.NewLiteral(-1), wantErr: true},
		{name: "fractional", literal: filter.NewLiteral(1.5), wantErr: true},
		{name: "oversized", literal: filter.NewLiteral(uint64(math.MaxUint64)), wantErr: true},
		{name: "invalid number", literal: &filter.Literal{Kind: filter.LiteralNumber, Value: "invalid"}, wantErr: true},
		{name: "bool", literal: filter.NewLiteral(true), wantErr: true},
		{name: "nil", literal: nil, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filtercompile.LiteralAsKey(tt.literal)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LiteralAsKey() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("LiteralAsKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLiteralToValue(t *testing.T) {
	tests := []struct {
		name    string
		literal *filter.Literal
		wantErr bool
	}{
		{name: "string", literal: filter.NewLiteral("lynx")},
		{name: "negative integer", literal: filter.NewLiteral(-1)},
		{name: "decimal", literal: filter.NewLiteral(1.5)},
		{name: "bool", literal: filter.NewLiteral(true)},
		{name: "nil", literal: nil, wantErr: true},
		{name: "null", literal: &filter.Literal{Kind: filter.LiteralNull, Value: "null"}, wantErr: true},
		{name: "invalid integer", literal: &filter.Literal{Kind: filter.LiteralNumber, Value: "-invalid"}, wantErr: true},
		{name: "invalid decimal", literal: &filter.Literal{Kind: filter.LiteralNumber, Value: "1e9999"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := filtercompile.LiteralToValue(tt.literal)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LiteralToValue() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestConversionRejectsMalformedOperands(t *testing.T) {
	if _, err := filtercompile.ExtractValue(filter.NewIdent("field")); err == nil {
		t.Fatal("ExtractValue accepted an identifier")
	}
	if _, err := filtercompile.CollectKeyPath(filter.NewLiteral("field")); err == nil {
		t.Fatal("CollectKeyPath accepted a literal")
	}
	invalidIndex := &filter.IndexExpr{
		Left:  filter.NewIdent("items"),
		Index: filter.NewLiteral(true),
	}
	if _, err := filtercompile.CollectKeyPath(invalidIndex); err == nil {
		t.Fatal("CollectKeyPath accepted a boolean index")
	}
}

func TestConvertListLiteralRejectsInvalidLists(t *testing.T) {
	tests := []struct {
		name string
		list *filter.ListLiteral
	}{
		{name: "nil", list: nil},
		{name: "empty", list: &filter.ListLiteral{}},
		{name: "nil first", list: &filter.ListLiteral{Values: []*filter.Literal{nil}}},
		{name: "mixed kinds", list: &filter.ListLiteral{Values: []*filter.Literal{filter.NewLiteral("a"), filter.NewLiteral(1)}}},
		{name: "invalid bool", list: &filter.ListLiteral{Values: []*filter.Literal{{Kind: filter.LiteralBool, Value: "invalid"}}}},
		{name: "null", list: &filter.ListLiteral{Values: []*filter.Literal{{Kind: filter.LiteralNull, Value: "null"}}}},
		{name: "invalid number", list: &filter.ListLiteral{Values: []*filter.Literal{{Kind: filter.LiteralNumber, Value: "invalid"}}}},
		{name: "decimal precision loss", list: &filter.ListLiteral{Values: []*filter.Literal{filter.NewLiteral(1.5), filter.NewLiteral(int64(1 << 54))}}},
		{name: "signed unsigned span", list: &filter.ListLiteral{Values: []*filter.Literal{filter.NewLiteral(uint64(math.MaxUint64)), filter.NewLiteral(-1)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := filtercompile.ConvertListLiteral(tt.list); err == nil {
				t.Fatal("ConvertListLiteral returned nil error")
			}
		})
	}
}

func TestRequireListLiteralRejectsEmptyList(t *testing.T) {
	expr := &filter.BinaryExpr{
		Left:  filter.NewIdent("field"),
		Op:    filter.OpIn,
		Right: &filter.ListLiteral{},
	}
	if _, err := filtercompile.RequireListLiteral(expr); err == nil {
		t.Fatal("RequireListLiteral accepted an empty list")
	}
}
