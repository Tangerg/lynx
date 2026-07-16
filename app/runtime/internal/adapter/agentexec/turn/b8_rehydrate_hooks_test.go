package turn_test

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestRehydrateRestoresCwdAndToolHooks(t *testing.T) {
	const (
		cwd       = "/restored/worktree"
		rewritten = `{"command":"echo restored"}`
	)
	recorder := &hookCommandRecorder{rewriteTool: "shell", rewriteArguments: rewritten}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.PreToolUse, Command: "record", Source: "test"},
	}, hooks.NewRunner(recorder, nil))
	engine := &stubEngine{
		restoreGateTool:      "shell",
		restoreGateArguments: `{"command":"echo original"}`,
	}
	dispatcher := mustTurn(turn.New(turnDeps(engine, func(deps *turn.Dependencies) {
		deps.Hooks = staticHookResolver{bound: bound}
	})))
	t.Cleanup(func() { _ = dispatcher.Close() })

	handle, err := dispatcher.Rehydrate(t.Context(), turn.RehydrateRequest{
		SessionID: "sess", TurnID: "turn", ProcessID: "process", Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	t.Cleanup(func() { _ = dispatcher.Cancel(t.Context(), handle) })

	engine.mu.Lock()
	verdict := engine.restoreGateVerdict
	engine.mu.Unlock()
	if verdict.Arguments != rewritten {
		t.Fatalf("restored gate arguments = %q, want %q", verdict.Arguments, rewritten)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.inputs) != 1 || recorder.inputs[0].Cwd != cwd {
		t.Fatalf("restored hook inputs = %#v, want cwd %q", recorder.inputs, cwd)
	}
}
