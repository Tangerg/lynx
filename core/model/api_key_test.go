package model_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model"
)

func TestApiKey_GetReturnsValue(t *testing.T) {
	const want = "sk-1234567890abcdef"
	k := model.NewApiKey(want)
	if got := k.Get(); got != want {
		t.Fatalf("Get = %q, want %q", got, want)
	}
}

func TestApiKey_StringNeverLeaksSecret(t *testing.T) {
	const secret = "sk-this-is-a-secret-token"
	k := model.NewApiKey(secret)

	stringer, ok := k.(interface{ String() string })
	if !ok {
		t.Fatal("ApiKey implementation must satisfy fmt.Stringer for safe logging")
	}

	masked := stringer.String()
	if strings.Contains(masked, "this-is-a-secret-token") {
		t.Fatalf("masked form leaks secret: %q", masked)
	}
	// Last 2 characters are intentionally exposed for at-a-glance checks.
	if !strings.HasSuffix(masked, "en") {
		t.Fatalf("masked form should keep the last two characters; got %q", masked)
	}
}

func TestApiKey_StringMaskingShapes(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		{name: "empty", key: "", want: "api_key=<empty>"},
		{name: "short three", key: "abc", want: "api_key=***"},
		{name: "boundary ten", key: "abcdefghij", want: "api_key=**********"},
		{name: "long", key: "sk-1234567890", want: "api_key=sk*********90"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := model.NewApiKey(tc.key)
			stringer := k.(interface{ String() string })
			if got := stringer.String(); got != tc.want {
				t.Fatalf("String = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApiKey_EmptyKeyForNoAuth(t *testing.T) {
	k := model.NewApiKey("")
	if got := k.Get(); got != "" {
		t.Fatalf("Get = %q, want empty for no-auth scenarios", got)
	}
}
