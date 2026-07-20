package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestTruthLogic(t *testing.T) {
	tests := []struct {
		name string
		a, b core.Truth
		want core.Truth
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
			var got core.Truth
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

func TestTruthNot(t *testing.T) {
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

func TestTruthZeroValueIsUnknown(t *testing.T) {
	var truth core.Truth
	if truth != core.Unknown {
		t.Fatalf("zero-value Truth should be Unknown, got %s", truth)
	}
}

func TestTruthRejectsValuesOutsideThreeValuedLogic(t *testing.T) {
	invalid := core.Truth(9)
	if invalid.Valid() {
		t.Fatal("invalid truth reported valid")
	}
	if got := invalid.String(); got != "invalid_truth(9)" {
		t.Fatalf("String() = %q", got)
	}
	if got := invalid.And(core.True); got != core.Unknown {
		t.Fatalf("invalid.And(true) = %v, want unknown", got)
	}
	if err := (core.ConditionSet{"ready": invalid}).Validate(); err == nil {
		t.Fatal("ConditionSet accepted invalid truth")
	}
}

func TestConditionSetValidateRejectsMalformedKeys(t *testing.T) {
	if err := (core.ConditionSet{"": core.True, " ready ": core.False}).Validate(); err == nil {
		t.Fatal("ConditionSet accepted malformed keys")
	}
}
