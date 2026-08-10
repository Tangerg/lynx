package dispatch

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

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
			When: []operation.FieldCondition{{
				Field: "type", Operator: operation.ConditionOperator(255),
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
