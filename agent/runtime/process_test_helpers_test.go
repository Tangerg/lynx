package runtime

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

// createProcessForTest registers a deliberately idle process for white-box
// lifecycle tests. Production fresh executions use admitProcessRun so their
// first run is owned before ProcessCreated becomes observable.
func createProcessForTest(
	t testing.TB,
	engine *Engine,
	agent *core.Agent,
	bindings core.Bindings,
	options core.ProcessOptions,
) *Process {
	t.Helper()
	deployment, err := engine.deploymentForProcess(t.Context(), agent)
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.buildProcessFromDeployment(deployment, bindings, options)
	if err != nil {
		t.Fatal(err)
	}
	if !engine.processes.insert(process) {
		t.Fatalf("register test process %q: duplicate ID", process.ID())
	}
	process.publishCreated(t.Context())
	return process
}
