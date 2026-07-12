package turn

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type controlFakeDispatcher struct {
	Dispatcher
	startReq        StartTurnRequest
	startHandle     TurnHandle
	steeringHandle  TurnHandle
	steeringMessage string
}

func (f *controlFakeDispatcher) StartTurn(_ context.Context, req StartTurnRequest) (TurnHandle, error) {
	f.startReq = req
	return f.startHandle, nil
}

func (f *controlFakeDispatcher) InjectSteering(_ context.Context, h TurnHandle, m string) error {
	f.steeringHandle = h
	f.steeringMessage = m
	return nil
}

type controlFakeSessions struct {
	resolved session.Session
	getID    string
	created  [2]string // title, cwd
	model    [2]string // id, model
	modelErr error
}

func (f *controlFakeSessions) Get(_ context.Context, id string) (session.Session, error) {
	f.getID = id
	return f.resolved, nil
}

func (f *controlFakeSessions) Create(_ context.Context, title, cwd string) (session.Session, error) {
	f.created = [2]string{title, cwd}
	return f.resolved, nil
}

func (f *controlFakeSessions) SetModel(_ context.Context, id, model string) error {
	f.model = [2]string{id, model}
	return f.modelErr
}

// TestControlPlanBindsToResolvedSession: an empty session id creates a session in
// defaultCwd; a non-empty one is fetched. The planned request inherits the
// resolved session's id + cwd.
func TestControlPlanBindsToResolvedSession(t *testing.T) {
	ctx := context.Background()
	sess := &controlFakeSessions{resolved: session.Session{ID: "ses_new", Cwd: "/w"}}
	c := NewControl(&controlFakeDispatcher{}, sess)

	got, planned, err := c.PlanTurnStart(ctx, "", "/default", StartTurnRequest{Message: "hi"})
	if err != nil {
		t.Fatalf("PlanTurnStart(new): %v", err)
	}
	if sess.created != ([2]string{"", "/default"}) {
		t.Fatalf("created = %v, want a session in /default", sess.created)
	}
	if got.ID != "ses_new" || planned.SessionID != "ses_new" || planned.Cwd != "/w" {
		t.Fatalf("planned = %+v (session %+v)", planned, got)
	}

	if _, _, err := c.PlanTurnStart(ctx, "ses_1", "/default", StartTurnRequest{Message: "hi"}); err != nil {
		t.Fatalf("PlanTurnStart(existing): %v", err)
	}
	if sess.getID != "ses_1" {
		t.Fatalf("get id = %q, want ses_1", sess.getID)
	}
}

// TestControlStartRecordsModelBeforeDispatch: an explicit model is persisted on
// the session before the turn dispatches.
func TestControlStartRecordsModelBeforeDispatch(t *testing.T) {
	handle := TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	disp := &controlFakeDispatcher{startHandle: handle}
	sess := &controlFakeSessions{}
	c := NewControl(disp, sess)

	got, err := c.StartTurn(context.Background(), StartTurnRequest{
		SessionID: "ses_1", Message: "hi", Provider: "anthropic", Model: "claude-opus-4-8",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if got != handle {
		t.Fatalf("handle = %+v, want %+v", got, handle)
	}
	if sess.model != ([2]string{"ses_1", "claude-opus-4-8"}) {
		t.Fatalf("session model = %v", sess.model)
	}
	if disp.startReq.Model != "claude-opus-4-8" || disp.startReq.Provider != "anthropic" {
		t.Fatalf("start req = %+v", disp.startReq)
	}
}

// TestControlStartDoesNotDispatchOnModelPersistFailure: if recording the model
// fails, the turn must not dispatch.
func TestControlStartDoesNotDispatchOnModelPersistFailure(t *testing.T) {
	fail := errors.New("store failed")
	disp := &controlFakeDispatcher{}
	c := NewControl(disp, &controlFakeSessions{modelErr: fail})

	if _, err := c.StartTurn(context.Background(), StartTurnRequest{SessionID: "ses_1", Message: "hi", Model: "claude-opus-4-8"}); !errors.Is(err, fail) {
		t.Fatalf("StartTurn err = %v, want store failure", err)
	}
	if disp.startReq.SessionID != "" {
		t.Fatalf("turn dispatched despite model persistence failure: %+v", disp.startReq)
	}
}

// TestControlInjectSteeringDelegates: steering is forwarded to the dispatcher.
func TestControlInjectSteeringDelegates(t *testing.T) {
	handle := TurnHandle{SessionID: "ses_1", TurnID: "run_1"}
	disp := &controlFakeDispatcher{}
	c := NewControl(disp, &controlFakeSessions{})

	if err := c.InjectTurnSteering(context.Background(), handle, "wait"); err != nil {
		t.Fatalf("InjectTurnSteering: %v", err)
	}
	if disp.steeringHandle != handle || disp.steeringMessage != "wait" {
		t.Fatalf("steering handle=%+v message=%q", disp.steeringHandle, disp.steeringMessage)
	}
}
