package dispatch

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestRegisteredShapesDescribeRealTypes is the whole point of declaring unions
// instead of inferring them: reflection cannot recover a discriminator, but it
// CAN check that a declaration matches the struct. Every registered spec is
// validated at init; this pins that the registrations exist and stay checked.
func TestRegisteredShapesDescribeRealTypes(t *testing.T) {
	t.Parallel()

	if len(shapes.Unions()) == 0 {
		t.Fatal("no union is registered")
	}
	for _, union := range shapes.Unions() {
		if err := union.validate(); err != nil {
			t.Errorf("union %s: %v", union.GoType.Name(), err)
		}
	}
	for _, constraint := range shapes.Constraints() {
		if err := constraint.validate(); err != nil {
			t.Errorf("constraint %s: %v", constraint.GoType.Name(), err)
		}
	}
	for _, key := range shapes.StateKeys() {
		if err := key.validate(); err != nil {
			t.Errorf("state key %s: %v", key.Key, err)
		}
	}
}

// TestEveryClosedWireUnionIsRegistered lists the unions that exist TODAY and
// must therefore have a spec. The four the contract also names — SegmentOutcome,
// ItemListScope, CapabilityRequirement, CancelRunResponse — arrive with the vNext
// cutover; registering a shape for a type nobody can send would check nothing.
func TestEveryClosedWireUnionIsRegistered(t *testing.T) {
	t.Parallel()

	want := []string{
		"ArtifactContentBlock", "ArtifactItem", "ArtifactOutcome",
		"ContentBlock", "Interrupt", "InterruptResponseValue", "Item", "ItemDelta",
		"QuestionField", "RunOutcome", "StreamEvent", "WorkspaceEvent",
	}
	got := make([]string, 0, len(shapes.Unions()))
	for _, union := range shapes.Unions() {
		got = append(got, union.GoType.Name())
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("registered unions = %v, want %v", got, want)
	}
}

// TestUnionValidationCatchesTheDriftItExistsFor covers the three ways a spec goes
// stale. The third is the one that actually happens: a field is added to the
// struct and no variant claims it, so a generated schema would permit it under
// every tag.
func TestUnionValidationCatchesTheDriftItExistsFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec UnionSpec
		want string
	}{{
		name: "a variant names a field the struct does not have",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: "text", Required: []string{"text"}},
				{Tag: "image", Required: []string{"mime", "data", "altText"}},
			},
		},
		want: `no JSON field "altText"`,
	}, {
		name: "a struct field belongs to no variant",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: "text", Required: []string{"text"}},
				{Tag: "image", Required: []string{"mime"}},
			},
		},
		want: `field "data" belongs to no variant`,
	}, {
		name: "a discriminator other than type",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "kind",
			Variants: []VariantSpec{{Tag: "text", Required: []string{"text"}}},
		},
		want: `discriminator is "kind"`,
	}, {
		name: "a duplicated tag",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: "text", Required: []string{"text", "mime", "data"}},
				{Tag: "text", Required: []string{"text"}},
			},
		},
		want: `variant "text" is declared twice`,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.spec.validate()
			if err == nil {
				t.Fatal("validate accepted a spec that does not match its struct")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestStateKeyMustHaveARecoveryMethodThatExists keeps the reconnect story real: a
// key whose recovery method is unregistered promises a call the client cannot
// make, which is worse than admitting there is no cold read.
func TestStateKeyMustHaveARecoveryMethodThatExists(t *testing.T) {
	t.Parallel()

	spec := StateKeySpec{
		Key: "todos", RecoveryMethod: "todos.get",
		Scope: StateScopeSession, Writer: StateWriterRootRun,
		Feature: "todos", Stability: stable,
	}
	if err := spec.validate(); err == nil {
		t.Fatal("validate accepted todos.get, which is not registered until C7")
	}
}
