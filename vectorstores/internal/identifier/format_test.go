package identifier

import (
	"strings"
	"testing"
)

func TestFormatMatch(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		value  string
		want   bool
	}{
		{name: "strict", format: Strict, value: "embedding_1", want: true},
		{name: "strict rejects dash", format: Strict, value: "embedding-field", want: false},
		{name: "dash", format: WithDash, value: "embedding-field", want: true},
		{name: "reject leading digit", format: WithDash, value: "1embedding", want: false},
		{name: "reject punctuation", format: WithDash, value: "embedding.field", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.format.Match(test.value); got != test.want {
				t.Fatalf("Match(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestFormatValidateUsesDeterministicFieldOrder(t *testing.T) {
	err := Strict.Validate("provider", map[string]string{
		"z_field": "invalid-value",
		"a_field": "also-invalid",
	})
	if err == nil || !strings.Contains(err.Error(), `a_field="also-invalid"`) {
		t.Fatalf("Validate() error = %v, want first invalid field by name", err)
	}
}
