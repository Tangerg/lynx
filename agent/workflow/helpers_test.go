package workflow_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// mustDeploy deploys agents on platform and fails the test on the first
// error. Lifts the unchecked-Deploy pattern out of the per-test setup
// blocks; equivalent to the explicit `if err := platform.Deploy(a); err != nil { t.Fatalf(...) }`
// shape used by older test files.
func mustDeploy(t *testing.T, p *runtime.Platform, agents ...*core.Agent) {
	t.Helper()
	for _, a := range agents {
		if err := p.Deploy(a); err != nil {
			t.Fatalf("deploy %q: %v", a.Name, err)
		}
	}
}

// mustOK is a tiny helper for "the constructor returned (T, error); fail
// the test on a non-nil error and return the T". Used in workflow tests
// to keep the happy-path call shape compact after the workflow.X
// constructors gained their error return.
func mustOK[T any](t *testing.T, v T, err error) T {
	t.Helper()
	if err != nil {
		t.Fatalf("workflow constructor: %v", err)
	}
	return v
}
