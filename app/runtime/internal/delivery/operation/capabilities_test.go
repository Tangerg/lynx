package operation

import (
	"reflect"
	"testing"
)

type conditionalParameters struct {
	Enabled bool   `json:"enabled,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Nested  *struct {
		Name string `json:"name,omitempty"`
	} `json:"nested,omitempty"`
}

func TestFieldConditionMatchesTypedParameters(t *testing.T) {
	t.Parallel()

	parameters := reflect.ValueOf(conditionalParameters{
		Enabled: true,
		Mode:    "files",
		Nested: &struct {
			Name string `json:"name,omitempty"`
		}{Name: "worker"},
	})
	tests := []struct {
		name      string
		condition FieldCondition
		want      bool
	}{
		{name: "present", condition: FieldCondition{Field: "enabled", Operator: OperatorPresent}, want: true},
		{name: "equals", condition: FieldCondition{Field: "mode", Operator: OperatorEquals, Value: "files"}, want: true},
		{name: "nested", condition: FieldCondition{Field: "nested.name", Operator: OperatorEquals, Value: "worker"}, want: true},
		{name: "empty", condition: FieldCondition{Field: "missing", Operator: OperatorPresent}},
		{name: "different", condition: FieldCondition{Field: "mode", Operator: OperatorEquals, Value: "history"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.condition.matches(parameters); got != test.want {
				t.Fatalf("matches = %v, want %v", got, test.want)
			}
		})
	}
}
