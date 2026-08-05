package turn_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestRehydrateRestoresCWDAndToolHooks(t *testing.T) {
	const (
		cwd       = "/restored/worktree"
		rewritten = `{"command":"echo restored","description":"Print restored"}`
	)
	recorder := &hookCommandRecorder{rewriteTool: "shell", rewriteArguments: rewritten}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.PreToolUse, Command: "record", Source: "test"},
	}, hooks.NewRunner(recorder, nil))
	engine := &stubEngine{
		restoreGateTool:      "shell",
		restoreGateArguments: `{"command":"echo original","description":"Print original"}`,
	}
	controller := mustTurn(turn.New(turnDeps(engine, func(deps *turn.Dependencies) {
		deps.Hooks = staticHookResolver{bound: bound}
	})))
	t.Cleanup(func() { shutdownController(t, controller) })

	handle, err := controller.Rehydrate(t.Context(), runs.RehydrateExecution{
		SessionID: "sess", ExecutorID: "turn", ProcessID: "process", RootRunID: "run-root", CWD: cwd,
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	t.Cleanup(func() { _ = controller.Cancel(t.Context(), handle) })

	engine.mu.Lock()
	verdict := engine.restoreGateVerdict
	engine.mu.Unlock()
	if verdict.Arguments != rewritten {
		t.Fatalf("restored gate arguments = %q, want %q", verdict.Arguments, rewritten)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.inputs) != 1 || recorder.inputs[0].CWD != cwd {
		t.Fatalf("restored hook inputs = %#v, want cwd %q", recorder.inputs, cwd)
	}
}

func TestRehydratePreservesHookResolutionFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	engine := &stubEngine{}
	controller := mustTurn(turn.New(turnDeps(engine, func(deps *turn.Dependencies) {
		deps.Hooks = staticHookResolver{err: wantErr}
	})))
	t.Cleanup(func() { shutdownController(t, controller) })

	if _, err := controller.Rehydrate(t.Context(), runs.RehydrateExecution{
		SessionID:  "sess",
		ExecutorID: "turn",
		ProcessID:  "process",
		RootRunID:  "run-root",
		CWD:        t.TempDir(),
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Rehydrate error = %v, want %v", err, wantErr)
	}
	if got := engine.restoreCalls.Load(); got != 0 {
		t.Fatalf("engine RestoreTurn calls = %d, want 0", got)
	}
}
