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
			Field: "runId", Kind: ConstraintKind("invalid"),
		}},
	}
	err := valueSpec.validate()
	if err == nil || !strings.Contains(err.Error(), `ConstraintKind("invalid")`) ||
		!strings.Contains(err.Error(), "GetRunRequest.runId") {
		t.Fatalf("value constraint error = %v, want shape, field and illegal kind", err)
	}
	if got := ConstraintKind("invalid").String(); got == ConstraintNonEmpty.String() {
		t.Fatalf("unknown constraint kind masquerades as %q", got)
	}

	bounded := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMinItems,
		}},
	}
	if validateErr := bounded.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "positive limit") {
		t.Fatalf("bounded constraint error = %v, want positive limit", validateErr)
	}

	unbounded := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.GetRunRequest](),
		Constraints: []FieldConstraint{{
			Field: "runId", Kind: ConstraintNonEmpty, Limit: 1,
		}},
	}
	if validateErr := unbounded.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "does not accept a limit") {
		t.Fatalf("unbounded constraint error = %v, want rejected limit", validateErr)
	}

	duplicateBound := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{
			{Field: "options", Kind: ConstraintMinItems, Limit: 2},
			{Field: "options", Kind: ConstraintMinItems, Limit: 3},
		},
	}
	if validateErr := duplicateBound.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "declares constraint minItems twice") {
		t.Fatalf("duplicate bounded constraint error = %v, want duplicate rejection", validateErr)
	}

	wrongType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMaxLength, Limit: 12,
		}},
	}
	if validateErr := wrongType.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "only a string has a length") {
		t.Fatalf("bounded constraint type error = %v, want string requirement", validateErr)
	}

	wrongMaximumType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMaximum, Limit: 3,
		}},
	}
	if validateErr := wrongMaximumType.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "only a number can have a maximum") {
		t.Fatalf("maximum constraint type error = %v, want numeric requirement", validateErr)
	}

	wrongMinimumType := FieldConstraintSpec{
		GoType: reflect.TypeFor[protocol.QuestionField](),
		Constraints: []FieldConstraint{{
			Field: "options", Kind: ConstraintMinimum, Limit: 3,
		}},
	}
	if validateErr := wrongMinimumType.validate(); validateErr == nil || !strings.Contains(validateErr.Error(), "only a number can have a minimum") {
		t.Fatalf("minimum constraint type error = %v, want numeric requirement", validateErr)
	}

	objectSpec := ObjectConstraintSpec{
		GoType: reflect.TypeFor[protocol.ProblemData](),
		Rules: []PresenceRule{{
			When: []operation.FieldCondition{{
				Field: "type", Operator: operation.ConditionOperator("invalid"),
			}},
			Required: []string{"detail"},
		}},
	}
	err = objectSpec.validate()
	if err == nil || !strings.Contains(err.Error(), "ProblemData") ||
		!strings.Contains(err.Error(), "type") ||
		!strings.Contains(err.Error(), `ConditionOperator("invalid")`) {
		t.Fatalf("object constraint error = %v, want shape, field and illegal operator", err)
	}

}
