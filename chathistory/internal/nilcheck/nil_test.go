package nilcheck

import "testing"

func TestIsNil(t *testing.T) {
	var pointer *int
	var slice []string
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil", want: true},
		{name: "typed pointer", value: pointer, want: true},
		{name: "typed slice", value: slice, want: true},
		{name: "value", value: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNil(test.value); got != test.want {
				t.Fatalf("IsNil(%T) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}
