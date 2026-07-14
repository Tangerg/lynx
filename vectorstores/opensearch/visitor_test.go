package opensearch_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/opensearch"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.Parse(src)
		if err != nil {
			return err
		}
		v := opensearch.NewVisitor("metadata")
		return v.Visit(expr)
	})
}

func TestVisitor_NullTest(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// A field is null when it is absent: negate the existence check.
			name: "is null",
			src:  "author is null",
			want: "NOT _exists_:metadata.author",
		},
		{
			// IS NOT NULL arrives as NOT(field IS NULL); the NOT wrapper
			// double-negates the existence check, leaving a plain exists.
			name: "is not null",
			src:  "author is not null",
			want: "NOT (NOT _exists_:metadata.author)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := filter.Parse(tt.src)
			if err != nil {
				t.Fatalf("parse %q: %v", tt.src, err)
			}
			v := opensearch.NewVisitor("metadata")
			if err := v.Visit(expr); err != nil {
				t.Fatalf("visit %q: %v", tt.src, err)
			}
			if got := v.Result(); got != tt.want {
				t.Errorf("Result() = %q, want %q", got, tt.want)
			}
		})
	}
}
