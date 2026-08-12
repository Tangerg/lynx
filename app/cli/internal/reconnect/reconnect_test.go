package reconnect

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestReconnectRetriesOnlyTransientErrorsWithinBudget(t *testing.T) {
	policy := Policy{Attempts: 3, Base: 10 * time.Millisecond, Maximum: 25 * time.Millisecond}
	for failure, want := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 25 * time.Millisecond} {
		got, ok := policy.Next(failure+1, agent.ErrDisconnected)
		if !ok || got != want {
			t.Fatalf("failure %d = %s, %v; want %s", failure+1, got, ok, want)
		}
	}
	if _, ok := policy.Next(4, agent.ErrDisconnected); ok {
		t.Fatal("retry budget was exceeded")
	}
	if _, ok := policy.Next(1, agent.ErrEventConflict); ok {
		t.Fatal("identity conflict was treated as transient")
	}
	if _, ok := policy.Next(1, agent.ErrReplayUnavailable); ok {
		t.Fatal("unavailable replay was treated as a retryable disconnect")
	}
}

func TestCommandCommitRetriesHonorTheRuntimeBackoffFloor(t *testing.T) {
	policy := Policy{Attempts: 2, Base: 10 * time.Millisecond, Maximum: 2 * time.Second}
	if delay, ok := policy.Next(1, agent.ErrCommandInProgress); !ok || delay != time.Second {
		t.Fatalf("command progress retry = %s, %t; want 1s, true", delay, ok)
	}
	if !Retryable(agent.ErrCommandInProgress) {
		t.Fatal("command progress was not retryable")
	}
	if Retryable(agent.ErrCommandConflict) {
		t.Fatal("command identity conflict was retryable")
	}
}

func TestRetryableRecognizesOnlyClassifiedDisconnects(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "disconnect", err: agent.ErrDisconnected, want: true},
		{name: "wrapped disconnect", err: fmt.Errorf("transport closed: %w", agent.ErrDisconnected), want: true},
		{name: "business error", err: errors.New("server not found")},
		{name: "compatibility error", err: agent.ErrIncompatibleRuntime},
		{name: "cancellation", err: context.Canceled},
		{name: "nil"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Retryable(test.err); got != test.want {
				t.Fatalf("Retryable(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}
