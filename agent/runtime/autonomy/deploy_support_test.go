package autonomy_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// mustDeploy deploys agents on platform and fails the test on the first error.
func mustDeploy(t *testing.T, p *runtime.Platform, agents ...*core.Agent) {
	t.Helper()
	for _, a := range agents {
		if err := p.Deploy(a); err != nil {
			t.Fatalf("deploy %q: %v", a.Name, err)
		}
	}
}
