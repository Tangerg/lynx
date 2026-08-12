package terminal

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
)

type sessionReadFailureRuntime struct {
	*mock.Runtime
	reads     atomic.Int32
	failureAt int32
}

func (runtime *sessionReadFailureRuntime) GetSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	if runtime.reads.Add(1) == runtime.failureAt {
		return agent.SessionSnapshot{}, agent.ErrDisconnected
	}
	return runtime.Runtime.GetSession(ctx, sessionID)
}

func TestRecoveredSessionRetriesATransientAttachRead(t *testing.T) {
	base := mock.New()
	base.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{
			Outcome: agent.Outcome{Status: agent.OutcomeCompleted},
		}}}}
	}
	_, err := base.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "recover attach"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &sessionReadFailureRuntime{Runtime: base, failureAt: 2}
	host, stop := runUIWithRuntimeChanges(t, runtime, nil, "ses_demo_1")
	host.Until(t, "the recovered session attach retry", func() bool {
		return runtime.reads.Load() >= 4 && host.Repaint()
	})
	host.Shows(t, "recover attach")
	stop()
}

func TestActiveDurationClockStartsFromDurableExecutionTime(t *testing.T) {
	startedAt := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	clock := activeDurationClock{}
	if got := clock.elapsed(startedAt); got != 0 {
		t.Fatalf("zero clock elapsed = %v, want zero", got)
	}

	clock.start(1400*time.Millisecond, startedAt)
	if got := clock.elapsed(startedAt.Add(600 * time.Millisecond)); got != 2*time.Second {
		t.Fatalf("resumed elapsed = %v, want 2s", got)
	}
	if got := clock.elapsed(startedAt.Add(-time.Second)); got != 1400*time.Millisecond {
		t.Fatalf("clock before local segment start = %v, want carried duration", got)
	}
}

func TestActiveDurationClockExcludesHumanWaitBetweenSegments(t *testing.T) {
	firstSegment := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	resume := firstSegment.Add(24 * time.Hour)
	clock := activeDurationClock{}
	clock.start(0, firstSegment)
	clock.start(3*time.Second, resume)

	if got := clock.elapsed(resume.Add(time.Second)); got != 4*time.Second {
		t.Fatalf("elapsed after overnight wait = %v, want 4s active time", got)
	}
}
