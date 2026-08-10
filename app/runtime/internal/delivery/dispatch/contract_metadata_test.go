package dispatch

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type registrySnapshotParams struct {
	Enabled bool `json:"enabled,omitempty"`
}

func TestRegistryViewsAreSnapshots(t *testing.T) {
	t.Parallel()

	registry := newRegistry()
	registry.add(MethodMeta{
		Name:        "tests.snapshot",
		Kind:        KindUnary,
		Operation:   OperationQuery,
		Idempotency: IdempotencyNone,
		Errors:      []string{protocol.ErrRunNotFound.Error()},
		CapabilityRules: []CapabilityRule{{
			When: []FieldCondition{{
				Field: "enabled", Operator: OperatorPresent,
			}},
			Requires: []string{protocol.FeatureSubagents},
		}},
		Stability: protocol.StabilityStable,
		Params:    reflect.TypeFor[registrySnapshotParams](),
	}, nil)

	metas := registry.Metas()
	metas[0].Errors[0] = protocol.ErrSessionNotFound.Error()
	metas[0].CapabilityRules[0].When[0].Field = "corrupted"
	metas[0].CapabilityRules[0].Requires[0] = protocol.FeatureKnowledge
	got := registry.Metas()[0]
	if !slices.Equal(got.Errors, []string{protocol.ErrRunNotFound.Error()}) {
		t.Fatalf("Metas exposed error storage: %v", got.Errors)
	}
	if got.CapabilityRules[0].When[0].Field != "enabled" {
		t.Fatalf("Metas exposed condition storage: %+v", got.CapabilityRules)
	}
	if !slices.Equal(got.CapabilityRules[0].Requires, []string{protocol.FeatureSubagents}) {
		t.Fatalf("Metas exposed requirement storage: %+v", got.CapabilityRules)
	}
}

func TestMetadataEnumsRejectUnknownValuesWithoutMasqueradingAsDefaults(t *testing.T) {
	t.Parallel()

	registered, ok := contract.lookup("runs.list")
	if !ok {
		t.Fatal("runs.list is not registered")
	}

	tests := []struct {
		name   string
		mutate func(*MethodMeta)
		want   []string
	}{{
		name: "method name",
		mutate: func(meta *MethodMeta) {
			meta.Name = "runs"
		},
		want: []string{`"runs"`, "dot-separated non-empty segments"},
	}, {
		name: "method kind",
		mutate: func(meta *MethodMeta) {
			meta.Kind = MethodKind(255)
		},
		want: []string{"runs.list", "MethodKind(255)"},
	}, {
		name: "operation kind",
		mutate: func(meta *MethodMeta) {
			meta.Operation = OperationKind(255)
		},
		want: []string{"runs.list", "OperationKind(255)"},
	}, {
		name: "idempotency policy",
		mutate: func(meta *MethodMeta) {
			meta.Idempotency = IdempotencyPolicy(255)
		},
		want: []string{"runs.list", "IdempotencyPolicy(255)"},
	}, {
		name: "pagination kind",
		mutate: func(meta *MethodMeta) {
			meta.Pagination = PaginationKind(255)
		},
		want: []string{"runs.list", "PaginationKind(255)"},
	}, {
		name: "pagination disagrees with shapes",
		mutate: func(meta *MethodMeta) {
			meta.Pagination = PaginationNone
		},
		want: []string{"runs.list", "shapes derive cursor"},
	}, {
		name: "stability",
		mutate: func(meta *MethodMeta) {
			meta.Stability = protocol.Stability("accidental")
		},
		want: []string{"runs.list", `"accidental"`, "stability"},
	}, {
		name: "condition operator",
		mutate: func(meta *MethodMeta) {
			meta.CapabilityRules = cloneCapabilityRules(meta.CapabilityRules)
			meta.CapabilityRules[0].When[0].Operator = ConditionOperator(255)
		},
		want: []string{"runs.list", "includeDescendants", "ConditionOperator(255)"},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := cloneMethodMeta(registered.Meta)
			tt.mutate(&meta)
			err := meta.validate()
			if err == nil {
				t.Fatal("validate accepted an unknown metadata enum")
			}
			for _, fragment := range tt.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %q, want fragment %q", err, fragment)
				}
			}
		})
	}

	if got := MethodKind(255).String(); got == KindUnary.String() || got == KindStream.String() {
		t.Fatalf("unknown method kind masquerades as %q", got)
	}
	if got := OperationKind(255).String(); got == OperationQuery.String() {
		t.Fatalf("unknown operation kind masquerades as %q", got)
	}
	if got := IdempotencyPolicy(255).String(); got == IdempotencyNone.String() {
		t.Fatalf("unknown idempotency policy masquerades as %q", got)
	}
	if got := PaginationKind(255).String(); got == PaginationNone.String() {
		t.Fatalf("unknown pagination kind masquerades as %q", got)
	}
	if got := ConditionOperator(255).String(); got == OperatorPresent.String() {
		t.Fatalf("unknown condition operator masquerades as %q", got)
	}
}

func TestPaginationIsDerivedFromWireShapes(t *testing.T) {
	t.Parallel()

	wantCursor := []string{
		"interrupts.list",
		"items.list",
		"runs.list",
		"schedules.list",
		"sessions.list",
		"workspace.files.list",
	}
	var gotCursor []string
	for _, method := range contract.Metas() {
		if method.Pagination == PaginationCursor {
			gotCursor = append(gotCursor, method.Name)
		}
	}
	slices.Sort(gotCursor)
	if !slices.Equal(gotCursor, wantCursor) {
		t.Fatalf("cursor-paginated methods = %v, want %v", gotCursor, wantCursor)
	}

	for _, name := range []string{"models.list", "providers.list", "runs.get", "tools.list", "workspaces.list"} {
		method, ok := contract.lookup(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if method.Meta.Pagination != PaginationNone {
			t.Errorf("%s pagination = %s, want %s", name, method.Meta.Pagination, PaginationNone)
		}
	}
}

func TestShapeMetadataRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	valueSpec := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintKind(255),
		}},
	}
	err := valueSpec.validate()
	if err == nil || !strings.Contains(err.Error(), "ConstraintKind(255)") ||
		!strings.Contains(err.Error(), "GetRunRequest.runId") {
		t.Fatalf("value constraint error = %v, want shape, field and illegal kind", err)
	}
	if got := ConstraintKind(255).String(); got == ConstraintNonEmpty.String() {
		t.Fatalf("unknown constraint kind masquerades as %q", got)
	}

	bounded := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMinItems,
		}},
	}
	if err := bounded.validate(); err == nil || !strings.Contains(err.Error(), "positive limit") {
		t.Fatalf("bounded constraint error = %v, want positive limit", err)
	}

	unbounded := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintNonEmpty, Limit: 1,
		}},
	}
	if err := unbounded.validate(); err == nil || !strings.Contains(err.Error(), "does not accept a limit") {
		t.Fatalf("unbounded constraint error = %v, want rejected limit", err)
	}

	duplicateBound := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{
			{Field: "options", Kind: ConstraintMinItems, Limit: 2},
			{Field: "options", Kind: ConstraintMinItems, Limit: 3},
		},
	}
	if err := duplicateBound.validate(); err == nil || !strings.Contains(err.Error(), "declares constraint minItems twice") {
		t.Fatalf("duplicate bounded constraint error = %v, want duplicate rejection", err)
	}

	wrongType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMaxLength, Limit: 12,
		}},
	}
	if err := wrongType.validate(); err == nil || !strings.Contains(err.Error(), "only a string has a length") {
		t.Fatalf("bounded constraint type error = %v, want string requirement", err)
	}

	wrongMaximumType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMaximum, Limit: 3,
		}},
	}
	if err := wrongMaximumType.validate(); err == nil || !strings.Contains(err.Error(), "only a number can have a maximum") {
		t.Fatalf("maximum constraint type error = %v, want numeric requirement", err)
	}

	wrongMinimumType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMinimum, Limit: 3,
		}},
	}
	if err := wrongMinimumType.validate(); err == nil || !strings.Contains(err.Error(), "only a number can have a minimum") {
		t.Fatalf("minimum constraint type error = %v, want numeric requirement", err)
	}

	objectSpec := ObjectConstraintSpec{
		GoType: reflect.TypeFor[protocol.ProblemData](),
		Rules: []PresenceRule{{
			When: []FieldCondition{{
				Field: "type", Operator: ConditionOperator(255),
			}},
			Required: []string{"detail"},
		}},
	}
	err = objectSpec.validate()
	if err == nil || !strings.Contains(err.Error(), "ProblemData") ||
		!strings.Contains(err.Error(), "type") ||
		!strings.Contains(err.Error(), "ConditionOperator(255)") {
		t.Fatalf("object constraint error = %v, want shape, field and illegal operator", err)
	}

	stateKey := shapes.StateKeys()[0]
	stateKey.Scope = StateSnapshotScope("workspace")
	err = stateKey.validate()
	if err == nil || !strings.Contains(err.Error(), "workspace") || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("state key error = %v, want illegal scope", err)
	}
}
