package filter_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestDispatchRejectsUnsupportedOperators(t *testing.T) {
	nullTest := mustParseBinary(t, `field is null`)
	if err := nullTest.Dispatch(filter.BinaryHandlers{}); err == nil {
		t.Fatal("Dispatch accepted a null operator")
	}
	if err := (*filter.UnaryExpr)(nil).Dispatch(nil); err == nil {
		t.Fatal("Dispatch accepted a nil unary expression")
	}
}

func TestLiteralKey(t *testing.T) {
	tests := []struct {
		name    string
		literal *filter.Literal
		want    string
		wantErr bool
	}{
		{name: "string", literal: filter.NewLiteral("name"), want: "name"},
		{name: "signed integer", literal: filter.NewLiteral(42), want: "42"},
		{name: "unsigned integer", literal: filter.NewLiteral(uint64(math.MaxInt64)), want: "9223372036854775807"},
		{name: "integral decimal", literal: filter.NewLiteral(4.0), want: "4"},
		{name: "negative", literal: filter.NewLiteral(-1), wantErr: true},
		{name: "fractional", literal: filter.NewLiteral(1.5), wantErr: true},
		{name: "oversized", literal: filter.NewLiteral(uint64(math.MaxUint64)), wantErr: true},
		{name: "bool", literal: filter.NewLiteral(true), wantErr: true},
		{name: "nil", literal: nil, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.literal.Key()
			if (err != nil) != test.wantErr {
				t.Fatalf("Key() error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("Key() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLiteralValue(t *testing.T) {
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
		{name: "non-finite", literal: filter.NewLiteral(math.Inf(1)), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.literal.Value()
			if (err != nil) != test.wantErr {
				t.Fatalf("Value() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestListValuesRejectInvalidLists(t *testing.T) {
	tests := []struct {
		name string
		list *filter.ListLiteral
	}{
		{name: "nil", list: nil},
		{name: "empty", list: filter.NewListLiteral([]int{})},
		{name: "nil first", list: filter.NewListLiteral([]*filter.Literal{nil})},
		{name: "mixed kinds", list: filter.NewListLiteral([]*filter.Literal{filter.NewLiteral("a"), filter.NewLiteral(1)})},
		{name: "decimal precision loss", list: filter.NewListLiteral([]*filter.Literal{filter.NewLiteral(1.5), filter.NewLiteral(int64(1 << 54))})},
		{name: "signed unsigned span", list: filter.NewListLiteral([]*filter.Literal{filter.NewLiteral(uint64(math.MaxUint64)), filter.NewLiteral(-1)})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.list.Values(); err == nil {
				t.Fatal("Values returned nil error")
			}
		})
	}
}

func TestExactNumberConversions(t *testing.T) {
	t.Run("exact text", func(t *testing.T) {
		literal := filter.NewLiteral(uint64(math.MaxUint64))
		actual, err := literal.NumberText()
		if err != nil || actual != "18446744073709551615" {
			t.Fatalf("NumberText() = %q, %v", actual, err)
		}
	})

	t.Run("int64 rejects fraction and overflow", func(t *testing.T) {
		if _, err := filter.NewLiteral(1.5).Int64(); err == nil {
			t.Fatal("Int64 accepted a fraction")
		}
		if _, err := filter.NewLiteral(uint64(math.MaxUint64)).Int64(); err == nil {
			t.Fatal("Int64 accepted uint64 overflow")
		}
	})

	t.Run("float64 rejects rounded integer", func(t *testing.T) {
		if _, err := filter.NewLiteral(uint64(1<<53 + 1)).Float64(); err == nil {
			t.Fatal("Float64 accepted a rounded integer")
		}
		if actual, err := filter.NewLiteral(uint64(1 << 54)).Float64(); err != nil || actual != 1<<54 {
			t.Fatalf("Float64(exact power of two) = %v, %v", actual, err)
		}
	})

	t.Run("float32 rejects rounded integer", func(t *testing.T) {
		if _, err := filter.NewLiteral(1<<24 + 1).Float32(); err == nil {
			t.Fatal("Float32 accepted a rounded integer")
		}
		if actual, err := filter.NewLiteral(1.5).Float32(); err != nil || actual != 1.5 {
			t.Fatalf("Float32(1.5) = %v, %v", actual, err)
		}
	})
}
