package agentexec

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
	"github.com/Tangerg/lynx/chatclient"
	toolcontract "github.com/Tangerg/lynx/tool"
)

type waitingDelegateFixture struct {
	model       *waitingDelegateModel
	executor    *InteractionExecutor
	coordinator *runs.Coordinator
	projection  *delegateProjection
}

func newWaitingDelegateFixture(t *testing.T, identity string) *waitingDelegateFixture {
	t.Helper()

	model := newWaitingDelegateModel()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	question, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "ask", Description: "Ask the user for the value required by delegated work.",
	}, waitingDelegateQuestion)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		DefaultClient:          client,
		ImplementationIdentity: identity + "-build",
		ConfigurationIdentity:  identity + "-config",
		DefaultMaxModelCalls:   4,
		BuildID:                interactionTestBuildID,
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{question},
		}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	if err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	sessions := &delegateSessionStore{value: sessionfixture.MustRestore(session.Snapshot{
		ID: "session_1", Title: "waiting delegate", CWD: workspace,
	})}
	projection := newDelegateProjection()
	runIDs := []string{"run_root", "run_child"}
	segmentIDs := []string{"segment_root", "segment_child"}
	coordinator := runs.NewCoordinator(runs.Dependencies{
		RootStarts: executor, Observations: executor, Releases: executor,
		Conversation: delegateConversation{},
		Session:      runs.SessionPorts{Reader: sessions, Creator: sessions, ActiveRuns: sessions},
		Projection: runs.ProjectionPorts{
			Openings: projection, ChildStarts: projection, Events: projection,
			Barriers: projection, Checkpoints: projection, Workspace: projection, Finalizer: projection,
		},
		Admissions: new(sessionadmission.Gate), Now: time.Now,
		NewRunID:     func() string { return takeFirstIdentifier(&runIDs) },
		NewSegmentID: func() string { return takeFirstIdentifier(&segmentIDs) },
	})
	return &waitingDelegateFixture{
		model: model, executor: executor, coordinator: coordinator, projection: projection,
	}
}

func waitingDelegateQuestion(ctx context.Context, _ struct{}) (string, error) {
	resolution, err := interactioninput.Require(ctx, "delegate.required-value", runs.Interrupt{
		Kind: interrupt.Question,
		Question: &runs.QuestionPrompt{
			ToolName: "ask", Arguments: `{}`,
			Fields: []runs.QuestionFieldSpec{{Prompt: "Which value should the delegate use?"}},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resolution.Answers) != 1 || len(resolution.Answers[0]) != 1 {
		return "", errors.New("delegated question returned an invalid answer shape")
	}
	return resolution.Answers[0][0], nil
}

func takeFirstIdentifier(identifiers *[]string) string {
	identifier := (*identifiers)[0]
	*identifiers = (*identifiers)[1:]
	return identifier
}

func (fixture *waitingDelegateFixture) start(t *testing.T) runs.StartResult {
	t.Helper()
	started, err := fixture.coordinator.Start(t.Context(), runs.StartCommand{
		SessionID: "session_1",
		Capabilities: run.Capabilities{
			ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question},
		},
		Input: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "delegate waiting work"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return started
}

func (fixture *waitingDelegateFixture) waitForBarrier(t *testing.T, timeout time.Duration) runs.TreeBarrierCommit {
	t.Helper()
	deadline := time.After(timeout)
	for {
		fixture.projection.mu.Lock()
		barriers := slices.Clone(fixture.projection.barriers)
		fixture.projection.mu.Unlock()
		if len(barriers) > 0 {
			if len(barriers) != 1 {
				t.Fatalf("waiting Delegate barriers = %d, want 1", len(barriers))
			}
			return barriers[0]
		}
		select {
		case <-deadline:
			t.Fatal("waiting Delegate did not publish a tree barrier")
		case <-time.After(time.Millisecond):
		}
	}
}

func (fixture *waitingDelegateFixture) admissionCounts() (reservations, outcomes int) {
	fixture.projection.mu.Lock()
	defer fixture.projection.mu.Unlock()
	return len(fixture.projection.reservations), len(fixture.projection.outcomes)
}

func (fixture *waitingDelegateFixture) shutdown(t *testing.T) {
	t.Helper()
	fixture.coordinator.BeginShutdown()
	if err := fixture.coordinator.AwaitShutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}
