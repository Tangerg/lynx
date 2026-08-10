package oneshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
)

type discardRenderer struct{}

func (discardRenderer) Render(agent.Envelope) error { return nil }
func (discardRenderer) Close() error                { return nil }

type countingRenderer struct {
	rendered int
	closed   int
}

func (r *countingRenderer) Render(agent.Envelope) error { r.rendered++; return nil }
func (r *countingRenderer) Close() error                { r.closed++; return nil }

type delayedStartResponse struct {
	*mock.Runtime
	accepted chan struct{}
}

func (r *delayedStartResponse) StartRun(ctx context.Context, input agent.StartRun) (agent.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return agent.Run{}, err
	}
	close(r.accepted)
	<-ctx.Done()
	return agent.Run{}, context.Cause(ctx)
}

type lostStartResponses struct{ *mock.Runtime }

func (r *lostStartResponses) StartRun(ctx context.Context, input agent.StartRun) (agent.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return agent.Run{}, err
	}
	return agent.Run{}, fmt.Errorf("start response lost: %w", agent.ErrDisconnected)
}

type invalidLifecycleRuntime struct {
	*mock.Runtime
	sessionID string
}

func (r *invalidLifecycleRuntime) StartRun(_ context.Context, input agent.StartRun) (agent.Run, error) {
	return agent.Run{ID: "run_invalid", SessionID: input.SessionID, Status: agent.RunActive}, nil
}

func (r *invalidLifecycleRuntime) FollowRun(_ context.Context, input agent.FollowRun) (agent.RunStream, error) {
	return func(yield func(agent.Envelope, error) bool) {
		if !yield(agent.Envelope{
			ID: "event_started", Cursor: input.After + 1,
			RunID: input.RunID, SessionID: r.sessionID,
			Event: agent.RunStarted{RunID: input.RunID, SessionID: r.sessionID},
		}, nil) {
			return
		}
		yield(agent.Envelope{
			ID: "event_invalid", Cursor: input.After + 2,
			RunID: input.RunID, SessionID: r.sessionID,
			Event: agent.RunStarted{RunID: input.RunID, SessionID: r.sessionID},
		}, nil)
	}, nil
}

type invalidStartRuntime struct{ *mock.Runtime }

func (r *invalidStartRuntime) StartRun(context.Context, agent.StartRun) (agent.Run, error) {
	return agent.Run{SessionID: "wrong", Status: agent.RunActive}, nil
}

func TestRunRejectsEventsThatViolateTheConversationLifecycle(t *testing.T) {
	base := mock.New()
	sessionID := firstSession(t, base)
	renderer := new(countingRenderer)
	err := Run(t.Context(), Config{
		Runtime:  &invalidLifecycleRuntime{Runtime: base, sessionID: sessionID},
		Renderer: renderer,
		Start: agent.StartRun{
			SessionID: sessionID,
			Message:   agent.Message{Text: "invalid lifecycle"},
		},
	})
	if !errors.Is(err, agent.ErrInvalidTransition) {
		t.Fatalf("run error = %v, want invalid transition", err)
	}
	if renderer.rendered != 1 {
		t.Fatalf("renderer received %d events, want only the valid prefix", renderer.rendered)
	}
	if renderer.closed != 1 {
		t.Fatalf("renderer closed %d times, want once", renderer.closed)
	}
}

func TestRunRequiresItsBoundaryDependencies(t *testing.T) {
	if err := Run(t.Context(), Config{Renderer: discardRenderer{}}); err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("missing runtime error = %v", err)
	}
	if err := Run(t.Context(), Config{Runtime: mock.New()}); err == nil || !strings.Contains(err.Error(), "renderer") {
		t.Fatalf("missing renderer error = %v", err)
	}
}

func TestRunRejectsAnInvalidStartProjection(t *testing.T) {
	base := mock.New()
	err := Run(t.Context(), Config{
		Runtime:  &invalidStartRuntime{Runtime: base},
		Renderer: discardRenderer{},
		Start: agent.StartRun{
			SessionID: firstSession(t, base),
			Message:   agent.Message{Text: "invalid start"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "start run response") {
		t.Fatalf("run error = %v", err)
	}
}

func TestRunCancellationTargetsAStartWhoseResponseWasLost(t *testing.T) {
	base := slowRuntime()
	sessionID := firstSession(t, base)
	runtime := &delayedStartResponse{Runtime: base, accepted: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		result <- Run(ctx, Config{
			Runtime: runtime, Renderer: discardRenderer{},
			Start: agent.StartRun{
				SessionID: sessionID,
				Message:   agent.Message{Text: "cancel after acceptance"},
			},
		})
	}()

	select {
	case <-runtime.accepted:
	case <-time.After(time.Second):
		t.Fatal("start was not accepted")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
	requireCanceledSession(t, base, sessionID)
}

func TestRunCancellationTargetsAStartAfterItsRetryBudgetIsExhausted(t *testing.T) {
	base := slowRuntime()
	sessionID := firstSession(t, base)
	err := Run(t.Context(), Config{
		Runtime: &lostStartResponses{Runtime: base}, Renderer: discardRenderer{},
		Start: agent.StartRun{
			SessionID: sessionID,
			Message:   agent.Message{Text: "lose every response"},
		},
		ReconnectAttempts: 1,
	})
	if !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("run error = %v, want exhausted transport failure", err)
	}
	requireCanceledSession(t, base, sessionID)
}

func slowRuntime() *mock.Runtime {
	runtime := mock.New()
	runtime.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour,
			Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	return runtime
}

func firstSession(t *testing.T, runtime agent.SessionCatalog) string {
	t.Helper()
	page, err := runtime.ListSessions(t.Context(), agent.SessionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items[0].ID
}

func requireCanceledSession(t *testing.T, runtime agent.SessionReader, sessionID string) {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil {
		t.Fatalf("canceled start left an active run: %+v", snapshot.Active)
	}
	finished, ok := snapshot.Events[len(snapshot.Events)-1].Event.(agent.RunFinished)
	if !ok || finished.Outcome.Status != agent.OutcomeCanceled {
		t.Fatalf("last event = %+v, want canceled run", snapshot.Events[len(snapshot.Events)-1])
	}
}
