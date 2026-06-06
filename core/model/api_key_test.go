package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model"
)

func TestApiKey_GetReturnsValue(t *testing.T) {
	const want = "sk-1234567890abcdef"
	k := model.NewAPIKey(want)
	if got := k.Get(); got != want {
		t.Fatalf("Get = %q, want %q", got, want)
	}
}

func TestApiKey_StringNeverLeaksSecret(t *testing.T) {
	const secret = "sk-this-is-a-secret-token"
	k := model.NewAPIKey(secret)

	stringer, ok := k.(interface{ String() string })
	if !ok {
		t.Fatal("APIKey implementation must satisfy fmt.Stringer for safe logging")
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
		{name: "empty", key: "", want: ""},
		{name: "short masked whole", key: "abc", want: "****"},
		{name: "boundary eight masked whole", key: "abcdefgh", want: "****"},
		{name: "nine shows ends", key: "abcdefghi", want: "ab****hi"},
		{name: "long fixed middle", key: "sk-1234567890", want: "sk****90"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := model.NewAPIKey(tc.key)
			stringer := k.(interface{ String() string })
			if got := stringer.String(); got != tc.want {
				t.Fatalf("String = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApiKey_EmptyKeyForNoAuth(t *testing.T) {
	k := model.NewAPIKey("")
	if got := k.Get(); got != "" {
		t.Fatalf("Get = %q, want empty for no-auth scenarios", got)
	}
}

// TestApiKey_MarshalJSONNeverLeaksSecret pins the contract that
// json-encoding a struct containing an APIKey emits the masked form,
// not the raw value. Without MarshalJSON, encoding/json reaches into
// unexported fields via reflection and would print the secret in plain
// text — Stringer alone does not protect this path.
func TestApiKey_MarshalJSONNeverLeaksSecret(t *testing.T) {
	const secret = "sk-very-sensitive-token"
	k := model.NewAPIKey(secret)

	out, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(out), "very-sensitive-token") {
		t.Fatalf("MarshalJSON leaked secret: %s", out)
	}
	// Sanity: containing struct passes through the masked form too.
	type wrapper struct {
		Key model.APIKey `json:"key"`
	}
	wrap, err := json.Marshal(wrapper{Key: k})
	if err != nil {
		t.Fatalf("Marshal wrapper: %v", err)
	}
	if strings.Contains(string(wrap), "very-sensitive-token") {
		t.Fatalf("wrapper MarshalJSON leaked secret: %s", wrap)
	}
}
