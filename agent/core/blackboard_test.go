package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type userInput struct{}
type HTTPResponse struct{}
type quote struct{}

func TestTypeKey(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "camel case", value: userInput{}, want: "user_input"},
		{name: "initialism", value: HTTPResponse{}, want: "http_response"},
		{name: "pointer", value: (*quote)(nil), want: "quote"},
		{name: "nil", value: nil, want: ""},
		{name: "anonymous", value: struct{}{}, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := core.TypeKey(test.value); got != test.want {
				t.Fatalf("TypeKey(%T) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
