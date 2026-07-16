package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestDeploymentRefValidateAndString(t *testing.T) {
	for _, test := range []struct {
		name    string
		ref     core.DeploymentRef
		want    string
		invalid bool
	}{
		{name: "unversioned", ref: core.DeploymentRef{Name: "writer", Digest: "digest"}, want: "writer@digest"},
		{name: "versioned", ref: core.DeploymentRef{Name: "writer", Version: "1.2.3", Digest: "digest"}, want: "writer@1.2.3+digest"},
		{name: "empty name", ref: core.DeploymentRef{Digest: "digest"}, invalid: true},
		{name: "empty digest", ref: core.DeploymentRef{Name: "writer"}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.ref.Validate()
			if test.invalid {
				if err == nil {
					t.Fatal("Validate returned nil")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := test.ref.String(); got != test.want {
				t.Fatalf("String = %q, want %q", got, test.want)
			}
		})
	}
}
