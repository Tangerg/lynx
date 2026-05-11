package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestDeterminationLogic(t *testing.T) {
	tests := []struct {
		name string
		a, b core.Determination
		want core.Determination
		op   string
	}{
		{"true and true", core.True, core.True, core.True, "and"},
		{"true and false", core.True, core.False, core.False, "and"},
		{"true and unknown", core.True, core.Unknown, core.Unknown, "and"},
		{"false dominates", core.False, core.Unknown, core.False, "and"},

		{"true or false", core.True, core.False, core.True, "or"},
		{"false or unknown", core.False, core.Unknown, core.Unknown, "or"},
		{"true or unknown", core.True, core.Unknown, core.True, "or"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got core.Determination
			switch tc.op {
			case "and":
				got = tc.a.And(tc.b)
			case "or":
				got = tc.a.Or(tc.b)
			}
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDeterminationNot(t *testing.T) {
	if core.True.Not() != core.False {
		t.Fatal("True.Not() != False")
	}
	if core.False.Not() != core.True {
		t.Fatal("False.Not() != True")
	}
	if core.Unknown.Not() != core.Unknown {
		t.Fatal("Unknown.Not() != Unknown")
	}
}

func TestDeterminationZeroValueIsUnknown(t *testing.T) {
	var d core.Determination
	if d != core.Unknown {
		t.Fatalf("zero-value Determination should be Unknown, got %s", d)
	}
}
