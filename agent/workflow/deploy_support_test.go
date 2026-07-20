package workflow_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// mustDeploy deploys agents on engine and fails the test on the first error.
func mustDeploy(t *testing.T, p *runtime.Engine, agents ...*core.Agent) {
	t.Helper()
	for _, a := range agents {
		if _, err := p.Deploy(t.Context(), a); err != nil {
			t.Fatalf("deploy %q: %v", a.Name(), err)
		}
	}
}
