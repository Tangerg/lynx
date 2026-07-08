package runtime

import (
	"context"
	"errors"
	"testing"

	agentcore "github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

func TestRuntimeCloseUsesCloserPort(t *testing.T) {
	closer := &fakeRuntimeCloser{}
	rt := &Runtime{closer: closer}

	if err := rt.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if !closer.closed {
		t.Fatal("closer port was not called")
	}
}

func TestRuntimeListSkillsUsesCatalogPort(t *testing.T) {
	catalog := &fakeSkillCatalog{
		skills: []kernel.SkillInfo{{Name: "lint", Description: "check code", Scope: "project"}},
	}
	rt := &Runtime{skillCatalog: catalog}

	got, err := rt.ListSkills(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("ListSkills err = %v", err)
	}
	if catalog.cwd != "/repo" {
		t.Fatalf("catalog cwd = %q", catalog.cwd)
	}
	if len(got) != 1 || got[0].Name != "lint" {
		t.Fatalf("skills = %+v", got)
	}
}

func TestRuntimeA2AAgentUsesChatRunnerPort(t *testing.T) {
	runner := &fakeChatRunner{
		process: newFakeTurnProcess("done"),
	}
	rt := &Runtime{a2aChats: runner}

	var chunks []string
	for chunk, err := range rt.A2AAgent().Run(context.Background(), "ship it") {
		if err != nil {
			t.Fatalf("Run yielded error: %v", err)
		}
		chunks = append(chunks, chunk)
	}

	if runner.message != "ship it" {
		t.Fatalf("runner message = %q", runner.message)
	}
	if len(chunks) != 1 || chunks[0] != "done" {
		t.Fatalf("chunks = %+v", chunks)
	}
}

type fakeRuntimeCloser struct {
	closed bool
}

func (f *fakeRuntimeCloser) Close() error {
	f.closed = true
	return nil
}

type fakeSkillCatalog struct {
	cwd    string
	skills []kernel.SkillInfo
}

func (f *fakeSkillCatalog) ListSkills(_ context.Context, cwd string) ([]kernel.SkillInfo, error) {
	f.cwd = cwd
	return f.skills, nil
}

type fakeChatRunner struct {
	message string
	process kernel.TurnProcess
}

func (f *fakeChatRunner) StartTurn(_ context.Context, req kernel.TurnRequest) kernel.TurnProcess {
	f.message = req.Message
	return f.process
}

type fakeTurnProcess struct {
	done   chan error
	output kernel.TurnOutput
}

func newFakeTurnProcess(reply string) *fakeTurnProcess {
	done := make(chan error, 1)
	done <- nil
	return &fakeTurnProcess{
		done:   done,
		output: kernel.TurnOutput{Reply: reply},
	}
}

func (f *fakeTurnProcess) ID() string { return "turn-1" }

func (f *fakeTurnProcess) Status() agentcore.AgentProcessStatus {
	return agentcore.StatusCompleted
}

func (f *fakeTurnProcess) Done() <-chan error { return f.done }

func (f *fakeTurnProcess) Output() (kernel.TurnOutput, error) { return f.output, nil }

func (f *fakeTurnProcess) Cancel() error { return errors.New("not implemented") }

func (f *fakeTurnProcess) Resume(context.Context, interrupts.Resolution) (<-chan error, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTurnProcess) PendingAwaitable() agentcore.Awaitable { return nil }

func (f *fakeTurnProcess) Discard(context.Context) {}
