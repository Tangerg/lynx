package ident

import (
	"strings"
	"testing"
)

func TestCheckReportsInvalidFieldsDeterministically(t *testing.T) {
	t.Parallel()

	err := Check("provider", map[string]string{
		"z_field": "also invalid",
		"a_field": "invalid value",
	})
	if err == nil {
		t.Fatal("Check() error = nil, want invalid identifier error")
	}
	if !strings.Contains(err.Error(), `a_field="invalid value"`) {
		t.Fatalf("Check() error = %q, want alphabetically first invalid field", err)
	}
}

func TestCheckWithDash(t *testing.T) {
	t.Parallel()

	if err := CheckWithDash("provider", map[string]string{"name": "valid-name"}); err != nil {
		t.Fatalf("CheckWithDash() error = %v, want nil", err)
	}
	if err := Check("provider", map[string]string{"name": "invalid-name"}); err == nil {
		t.Fatal("Check() error = nil, want dash rejection")
	}
}
