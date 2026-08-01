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
	if len(shapes.Notifications()) == 0 {
		t.Fatal("no downstream notification is registered")
	}
	for _, notification := range shapes.Notifications() {
		if err := notification.validate(); err != nil {
			t.Errorf("notification %s: %v", notification.Name, err)
		}
	}
}

func TestShapeViewsAreSnapshots(t *testing.T) {
	t.Parallel()

	unions := shapes.Unions()
	originalTag := unions[0].Variants[0].Tag
	unions[0].Variants[0].Tag = "corrupted"
	if got := shapes.Unions()[0].Variants[0].Tag; got != originalTag {
		t.Fatalf("Unions exposed registry storage: got %q, want %q", got, originalTag)
	}

	values := shapes.ValueConstraints()
	originalField := values[0].Constraints[0].Field
	values[0].Constraints[0].Field = "corrupted"
	if got := shapes.ValueConstraints()[0].Constraints[0].Field; got != originalField {
		t.Fatalf("ValueConstraints exposed registry storage: got %q, want %q", got, originalField)
	}

	stateKeys := shapes.StateKeys()
	originalKey := stateKeys[0].Key
	stateKeys[0].Key = "corrupted"
	if got := shapes.StateKeys()[0].Key; got != originalKey {
		t.Fatalf("StateKeys exposed registry storage: got %q, want %q", got, originalKey)
	}

	notifications := shapes.Notifications()
	originalName := notifications[0].Name
	notifications[0].Name = "notifications.corrupted"
	if got := shapes.Notifications()[0].Name; got != originalName {
		t.Fatalf("Notifications exposed registry storage: got %q, want %q", got, originalName)
	}
}

func TestNotificationValidationRejectsUnpublishableParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec NotificationSpec
		want string
	}{
		{name: "empty method", spec: NotificationSpec{}, want: "method name"},
		{
			name: "missing params",
			spec: NotificationSpec{Name: "notifications.test.event"},
			want: "no params type",
		},
		{
			name: "anonymous params",
			spec: NotificationSpec{
				Name: "notifications.test.event", ParamsType: reflect.TypeFor[struct{}](),
			},
			want: "named struct",
		},
		{
			name: "wrong namespace",
			spec: NotificationSpec{
				Name: "runtime.event", ParamsType: reflect.TypeFor[protocol.RuntimeEventNotification](),
			},
			want: "notifications namespace",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.spec.validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want it to mention %q", err, test.want)
			}
		})
	}
}

// TestEveryClosedWireUnionIsRegistered lists every closed union in the frozen
// contract. A new wire union does not exist until both its Go carrier and its
// machine-readable UnionSpec land.
//
// DiffRow is not one of the thirteen §11.2 names: its godoc always described a
// union and the frontend always modeled it as one, but nothing on the wire said
// so, so the published shape allowed a row with a hunk's text and both line
// numbers. Generating the TypeScript is what surfaced it.
func TestEveryClosedWireUnionIsRegistered(t *testing.T) {
	t.Parallel()

	want := []string{
		"ArtifactContentBlock", "ArtifactItem", "ArtifactOutcome", "ArtifactState",
		"CancelRunResponse", "CapabilityRequirement", "ContentBlock", "DiffRow", "Interrupt", "InterruptResponseValue", "Item", "ItemDelta",
		"ItemListScope", "QuestionField", "RunOutcome", "RuntimeEvent", "SegmentOutcome", "StateSnapshot", "StreamEvent",
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

	validContentBlockVariants := []VariantSpec{
		{Tag: "text", Required: []string{"text"}},
		{Tag: "image", Required: []string{"mime", "data"}},
	}
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
	}, {
		name: "an empty forbidden field",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{""},
		},
		want: `forbidden field 0 has no name`,
	}, {
		name: "a nested forbidden field",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{"legacy.value"},
		},
		want: `forbidden field "legacy.value" must be a top-level JSON member`,
	}, {
		name: "a duplicated forbidden field",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{"legacy", "legacy"},
		},
		want: `forbidden field "legacy" is declared twice`,
	}, {
		name: "a forbidden field still on the wire shape",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{"data"},
		},
		want: `forbidden field "data" still exists on the Go wire shape`,
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

func TestUnionValidationAcceptsRemovedForbiddenField(t *testing.T) {
	t.Parallel()

	spec := UnionSpec{
		GoType:        reflect.TypeFor[protocol.ContentBlock](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: "text", Required: []string{"text"}},
			{Tag: "image", Required: []string{"mime", "data"}},
		},
		Forbidden: []string{"legacy"},
	}
	if err := spec.validate(); err != nil {
		t.Fatalf("validate rejected a protocol-level negative invariant: %v", err)
	}
}

// TestStateKeyMustHaveARecoveryMethodThatExists keeps the reconnect story real: a
// key whose recovery method is unregistered promises a call the client cannot
// make, which is worse than admitting there is no cold read.
func TestStateKeyMustHaveARecoveryMethodThatExists(t *testing.T) {
	t.Parallel()

	spec := StateKeySpec{
		Key: "todos", RecoveryMethod: "todos.fetch",
		Scope: StateScopeSession, Writer: StateWriterRootRun,
		Feature: "todos", Stability: stable,
	}
	if err := spec.validate(); err == nil {
		t.Fatal("validate accepted a recovery method no registration serves")
	}
}
