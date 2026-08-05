package nilvalue_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
)

func TestIs(t *testing.T) {
	var pointer *int
	var function func()
	var channel chan int
	var slice []int
	var mapping map[string]int

	for _, test := range []struct {
		name  string
		value any
		want  bool
	}{
		{name: "untyped nil", value: nil, want: true},
		{name: "typed nil pointer", value: pointer, want: true},
		{name: "typed nil function", value: function, want: true},
		{name: "typed nil channel", value: channel, want: true},
		{name: "typed nil slice", value: slice, want: true},
		{name: "typed nil map", value: mapping, want: true},
		{name: "zero integer", value: 0, want: false},
		{name: "empty string", value: "", want: false},
		{name: "non-nil pointer", value: new(int), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nilvalue.Is(test.value); got != test.want {
				t.Fatalf("Is(%T) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}
