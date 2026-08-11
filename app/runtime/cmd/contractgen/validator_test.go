package main

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
)

type validatorUnionFixture struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Alpha string `json:"alpha,omitempty"`
	Beta  string `json:"beta,omitempty"`
}

func TestUnionBranchPresenceDoesNotBecomeGlobalRequiredness(t *testing.T) {
	t.Parallel()

	shape := reflect.TypeFor[validatorUnionFixture]()
	checks := validatorChecks(
		shape,
		[]dispatch.FieldConstraint{
			{Field: "id", Kind: dispatch.ConstraintNonEmpty},
			{Field: "alpha", Kind: dispatch.ConstraintNonEmpty},
			{Field: "beta", Kind: dispatch.ConstraintNonEmpty},
		},
		dispatch.UnionSpec{
			GoType: shape, Discriminator: "type",
			Variants: []dispatch.VariantSpec{
				{Tag: "alpha", Required: []string{"id", "alpha"}},
				{Tag: "beta", Required: []string{"id", "beta"}},
			},
		},
		nil,
	)

	if !slices.Contains(checks, `requiredText("id", value.ID)`) {
		t.Fatalf("non-optional identity lost its value constraint: %v", checks)
	}
	for _, forbidden := range []string{
		`requiredText("alpha", value.Alpha)`,
		`requiredText("beta", value.Beta)`,
	} {
		if slices.Contains(checks, forbidden) {
			t.Fatalf("branch field became globally required through %s", forbidden)
		}
	}
	for _, required := range []string{
		`requiredWhen(wireFieldEquals(value, "type", "alpha"), "alpha", value)`,
		`requiredWhen(wireFieldEquals(value, "type", "beta"), "beta", value)`,
	} {
		if !slices.Contains(checks, required) {
			t.Fatalf("branch requiredness was not generated through %s: %v", required, checks)
		}
	}
}
