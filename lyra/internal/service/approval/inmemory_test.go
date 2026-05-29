package approval_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
)

// TestRoundTrip exercises the producer/consumer flow: Register
// returns a channel, Decide pushes onto it, the producer reads
// the decision.
func TestRoundTrip(t *testing.T) {
	svc := approval.New(approval.ModeSafe)
	req := approval.Request{ID: "r1", ToolName: "bash", Arguments: "{}"}

	decisionCh, cleanup := svc.Register(req)
	defer cleanup()

	// Now that the request is registered, Decide must succeed.
	if err := svc.Decide(context.Background(), "r1", approval.DecisionApprove); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	select {
	case got := <-decisionCh:
		if got != approval.DecisionApprove {
			t.Errorf("decision = %v, want AllowOnce", got)
		}
	case <-time.After(time.Second):
		t.Fatal("decision channel never delivered")
	}
}

// TestDecideUnknown returns ErrRequestNotFound when the id has
// no matching pending entry.
func TestDecideUnknown(t *testing.T) {
	svc := approval.New(approval.ModeSafe)
	err := svc.Decide(context.Background(), "no-such", approval.DecisionApprove)
	if !errors.Is(err, approval.ErrRequestNotFound) {
		t.Errorf("err = %v, want ErrRequestNotFound", err)
	}
}

// TestCleanupClearsPending — after cleanup the pending entry must
// be gone so a subsequent Decide on the same id is a 404.
func TestCleanupClearsPending(t *testing.T) {
	svc := approval.New(approval.ModeSafe)
	_, cleanup := svc.Register(approval.Request{ID: "x", ToolName: "bash"})
	pending, _ := svc.ListPending(context.Background())
	if len(pending) != 1 {
		t.Fatalf("post-Register pending count = %d, want 1", len(pending))
	}
	cleanup()
	pending, _ = svc.ListPending(context.Background())
	if len(pending) != 0 {
		t.Errorf("post-cleanup pending count = %d, want 0", len(pending))
	}
	if err := svc.Decide(context.Background(), "x", approval.DecisionApprove); !errors.Is(err, approval.ErrRequestNotFound) {
		t.Errorf("post-cleanup Decide err = %v, want ErrRequestNotFound", err)
	}
}

// TestModeGetSet round-trips the mode atomic.
func TestModeGetSet(t *testing.T) {
	svc := approval.New(approval.ModeYolo)
	if m, _ := svc.GetMode(context.Background()); m != approval.ModeYolo {
		t.Errorf("initial mode = %v, want Yolo", m)
	}
	_ = svc.SetMode(context.Background(), approval.ModeBalanced)
	if m, _ := svc.GetMode(context.Background()); m != approval.ModeBalanced {
		t.Errorf("after Set, mode = %v, want Balanced", m)
	}
}

// TestListPending returns every outstanding request and stops
// listing them once cleanup runs.
func TestListPending(t *testing.T) {
	svc := approval.New(approval.ModeSafe)
	cleanups := make([]func(), 0, 2)
	for _, id := range []string{"a", "b"} {
		_, cleanup := svc.Register(approval.Request{ID: id, ToolName: "bash"})
		cleanups = append(cleanups, cleanup)
	}

	pending, _ := svc.ListPending(context.Background())
	if len(pending) != 2 {
		t.Errorf("pending count = %d, want 2", len(pending))
	}

	for _, c := range cleanups {
		c()
	}
	pending, _ = svc.ListPending(context.Background())
	if len(pending) != 0 {
		t.Errorf("after cleanup all, pending = %d, want 0", len(pending))
	}
}
