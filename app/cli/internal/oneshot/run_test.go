package oneshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/client/mock"
)

type discardRenderer struct{}

func (discardRenderer) Render(client.Envelope) error { return nil }
func (discardRenderer) Close() error                 { return nil }

type countingRenderer struct {
	rendered int
	closed   int
}

func (r *countingRenderer) Render(client.Envelope) error { r.rendered++; return nil }
func (r *countingRenderer) Close() error                 { r.closed++; return nil }

type delayedStartResponse struct {
	*mock.Runtime
	accepted chan struct{}
}

func (r *delayedStartResponse) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return client.Run{}, err
	}
	close(r.accepted)
	<-ctx.Done()
	return client.Run{}, context.Cause(ctx)
}

type lostStartResponses struct{ *mock.Runtime }

func (r *lostStartResponses) StartRun(ctx context.Context, input client.StartRun) (client.Run, error) {
	if _, err := r.Runtime.StartRun(ctx, input); err != nil {
		return client.Run{}, err
	}
	return client.Run{}, fmt.Errorf("start response lost: %w", client.ErrDisconnected)
}

type invalidLifecycleRuntime struct {
	*mock.Runtime
	sessionID string
}

func (r *invalidLifecycleRuntime) StartRun(_ context.Context, input client.StartRun) (client.Run, error) {
	return client.Run{ID: "run_invalid", SessionID: input.SessionID, Status: client.RunActive}, nil
}

func (r *invalidLifecycleRuntime) FollowRun(_ context.Context, input client.FollowRun) (client.Stream, error) {
	return func(yield func(client.Envelope, error) bool) {
		if !yield(client.Envelope{
			ID: "event_started", Cursor: input.After + 1,
			RunID: input.RunID, SessionID: r.sessionID,
			Event: client.RunStarted{RunID: input.RunID, SessionID: r.sessionID},
		}, nil) {
			return
		}
		yield(client.Envelope{
			ID: "event_invalid", Cursor: input.After + 2,
			RunID: input.RunID, SessionID: r.sessionID,
			Event: client.RunStarted{RunID: input.RunID, SessionID: r.sessionID},
		}, nil)
	}, nil
}

type invalidStartRuntime struct{ *mock.Runtime }

func (r *invalidStartRuntime) StartRun(context.Context, client.StartRun) (client.Run, error) {
	return client.Run{SessionID: "wrong", Status: client.RunActive}, nil
}

func TestRunRejectsEventsThatViolateTheConversationLifecycle(t *testing.T) {
	base := mock.New()
	sessionID := firstSession(t, base)
	renderer := new(countingRenderer)
	err := Run(t.Context(), Config{
		Runtime:  &invalidLifecycleRuntime{Runtime: base, sessionID: sessionID},
		Renderer: renderer,
		Start: client.StartRun{
			SessionID: sessionID,
			Message:   client.Message{Text: "invalid lifecycle"},
		},
	})
	if !errors.Is(err, client.ErrInvalidTransition) {
		t.Fatalf("run error = %v, want invalid transition", err)
	}
	if renderer.rendered != 1 {
		t.Fatalf("renderer received %d events, want only the valid prefix", renderer.rendered)
	}
	if renderer.closed != 1 {
		t.Fatalf("renderer closed %d times, want once", renderer.closed)
	}
}

func TestRunRejectsAnInvalidStartProjection(t *testing.T) {
	base := mock.New()
	err := Run(t.Context(), Config{
		Runtime:  &invalidStartRuntime{Runtime: base},
		Renderer: discardRenderer{},
		Start: client.StartRun{
			SessionID: firstSession(t, base),
			Message:   client.Message{Text: "invalid start"},
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
			Start: client.StartRun{
				SessionID: sessionID,
				Message:   client.Message{Text: "cancel after acceptance"},
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
		Start: client.StartRun{
			SessionID: sessionID,
			Message:   client.Message{Text: "lose every response"},
		},
		ReconnectAttempts: 1,
	})
	if !errors.Is(err, client.ErrDisconnected) {
		t.Fatalf("run error = %v, want exhausted transport failure", err)
	}
	requireCanceledSession(t, base, sessionID)
}

func slowRuntime() *mock.Runtime {
	runtime := mock.New()
	runtime.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{
			Delay: time.Hour,
			Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}},
		}}}
	}
	return runtime
}

func firstSession(t *testing.T, runtime client.SessionCatalog) string {
	t.Helper()
	page, err := runtime.ListSessions(t.Context(), client.SessionQuery{})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items[0].ID
}

func requireCanceledSession(t *testing.T, runtime client.SessionReader, sessionID string) {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil {
		t.Fatalf("canceled start left an active run: %+v", snapshot.Active)
	}
	finished, ok := snapshot.Events[len(snapshot.Events)-1].Event.(client.RunFinished)
	if !ok || finished.Outcome.Status != client.OutcomeCanceled {
		t.Fatalf("last event = %+v, want canceled run", snapshot.Events[len(snapshot.Events)-1])
	}
}
