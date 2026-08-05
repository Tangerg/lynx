package mock

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

// instant builds a mock that plays scripts with no delay, and the id of a
// session to run in.
func instant(t *testing.T) (*Runtime, string) {
	t.Helper()
	rt := New()
	rt.Instant = true
	sessions, err := rt.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("mock seeded no sessions")
	}
	return rt, sessions[0].ID
}

// drain collects a stream, returning the events and the first error.
func drain(s client.Stream) ([]client.Event, error) {
	var out []client.Event
	for ev, err := range s {
		if err != nil {
			return out, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func kinds(events []client.Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		switch e := ev.(type) {
		case client.RunStarted:
			out = append(out, "run.started")
		case client.BlockStarted:
			out = append(out, "block.started:"+string(e.Block.Kind))
		case client.BlockDelta:
			out = append(out, "block.delta")
		case client.BlockCompleted:
			out = append(out, "block.completed:"+string(e.Block.Kind))
		case client.PlanChanged:
			out = append(out, "plan.changed")
		case client.RunParked:
			out = append(out, "run.parked")
		case client.RunFinished:
			out = append(out, "run.finished:"+string(e.Outcome.Status))
		}
	}
	return out
}

func TestStartRunOpensWithIdentityAndParks(t *testing.T) {
	rt, session := instant(t)

	stream, err := rt.StartRun(context.Background(), client.StartRun{SessionID: session, Prompt: "why?"})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	events, err := drain(stream)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("stream produced nothing")
	}
	started, ok := events[0].(client.RunStarted)
	if !ok {
		t.Fatalf("first event is %T, want client.RunStarted", events[0])
	}
	if started.RunID == "" || started.SessionID != session {
		t.Fatalf("RunStarted = %+v, want a run id in session %s", started, session)
	}
	if _, ok := events[len(events)-1].(client.RunParked); !ok {
		t.Fatalf("last event is %T, want client.RunParked", events[len(events)-1])
	}
	// A parked run has not finished; anything that reports otherwise would send a
	// caller home early.
	for _, ev := range events {
		if _, ok := ev.(client.RunFinished); ok {
			t.Fatal("prelude reported a finished run before the park was answered")
		}
	}
}

func TestDeltasConcatenateIntoTheCompletedBody(t *testing.T) {
	rt, session := instant(t)

	stream, _ := rt.StartRun(context.Background(), client.StartRun{SessionID: session, Prompt: "why?"})
	events, err := drain(stream)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}

	built := map[string]*strings.Builder{}
	checked := 0
	for _, ev := range events {
		switch e := ev.(type) {
		case client.BlockStarted:
			b := &strings.Builder{}
			b.WriteString(e.Block.Text)
			built[e.Block.ID] = b
		case client.BlockDelta:
			if b, ok := built[e.BlockID]; ok {
				b.WriteString(e.Text)
			}
		case client.BlockCompleted:
			b, ok := built[e.Block.ID]
			if !ok {
				continue
			}
			if got := b.String(); got != e.Block.Text {
				t.Fatalf("block %s: deltas built %q, completed body is %q", e.Block.ID, got, e.Block.Text)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no streamed block was checked; the script no longer streams anything")
	}
}

func TestResumeApprovedAndDeniedBothFinish(t *testing.T) {
	for _, tc := range []struct {
		name     string
		approved bool
	}{
		{"approved", true},
		{"denied", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, session := instant(t)
			stream, _ := rt.StartRun(context.Background(), client.StartRun{SessionID: session, Prompt: "why?"})
			events, err := drain(stream)
			if err != nil {
				t.Fatalf("drain: %v", err)
			}
			runID := events[0].(client.RunStarted).RunID
			parked := events[len(events)-1].(client.RunParked)

			cont, err := rt.ResumeRun(context.Background(), client.ResumeRun{
				RunID:       runID,
				InterruptID: parked.Approval.InterruptID,
				Decision:    client.Decision{Approved: tc.approved},
			})
			if err != nil {
				t.Fatalf("ResumeRun: %v", err)
			}
			after, err := drain(cont)
			if err != nil {
				t.Fatalf("drain continuation: %v", err)
			}
			last, ok := after[len(after)-1].(client.RunFinished)
			if !ok {
				t.Fatalf("continuation ends with %T, want client.RunFinished", after[len(after)-1])
			}
			if last.Outcome.Status != client.OutcomeCompleted {
				t.Fatalf("outcome = %s, want completed", last.Outcome.Status)
			}
			if last.Usage.InputTokens == 0 {
				t.Fatal("finished run reported no usage")
			}

			edited := strings.Contains(strings.Join(kinds(after), " "), "block.completed:tool")
			if edited != tc.approved {
				t.Fatalf("tool ran = %v, want %v", edited, tc.approved)
			}
		})
	}
}

func TestResumeRejectsAnAnswerToTheWrongInterrupt(t *testing.T) {
	rt, session := instant(t)
	stream, _ := rt.StartRun(context.Background(), client.StartRun{SessionID: session, Prompt: "why?"})
	events, _ := drain(stream)
	runID := events[0].(client.RunStarted).RunID

	if _, err := rt.ResumeRun(context.Background(), client.ResumeRun{
		RunID:       runID,
		InterruptID: "int_does_not_exist",
	}); !errors.Is(err, client.ErrInterruptNotOpen) {
		t.Fatalf("err = %v, want ErrInterruptNotOpen", err)
	}
}

func TestResumeTwiceIsRefused(t *testing.T) {
	rt, session := instant(t)
	stream, _ := rt.StartRun(context.Background(), client.StartRun{SessionID: session, Prompt: "why?"})
	events, _ := drain(stream)
	runID := events[0].(client.RunStarted).RunID
	parked := events[len(events)-1].(client.RunParked)
	resume := client.ResumeRun{RunID: runID, InterruptID: parked.Approval.InterruptID, Decision: client.Decision{Approved: true}}

	first, err := rt.ResumeRun(context.Background(), resume)
	if err != nil {
		t.Fatalf("first ResumeRun: %v", err)
	}
	if _, err := drain(first); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if _, err := rt.ResumeRun(context.Background(), resume); !errors.Is(err, client.ErrInterruptNotOpen) {
		t.Fatalf("second ResumeRun err = %v, want ErrInterruptNotOpen", err)
	}
}

func TestStartRunRejectsAnUnknownSession(t *testing.T) {
	rt, _ := instant(t)
	if _, err := rt.StartRun(context.Background(), client.StartRun{SessionID: "ses_nope", Prompt: "why?"}); !errors.Is(err, client.ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestCancelEndsTheRunAsCanceled(t *testing.T) {
	rt := New()
	rt.Instant = true
	sessions, _ := rt.ListSessions(context.Background())

	stream, err := rt.StartRun(context.Background(), client.StartRun{SessionID: sessions[0].ID, Prompt: "why?"})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	// Cancel after the run has announced itself, mid-stream, the way a Ctrl-C
	// arrives.
	var events []client.Event
	var canceled bool
	for ev, err := range stream {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		events = append(events, ev)
		if started, ok := ev.(client.RunStarted); ok && !canceled {
			canceled = true
			if err := rt.CancelRun(context.Background(), started.RunID); err != nil {
				t.Fatalf("CancelRun: %v", err)
			}
			// A second cancel must be harmless: a doubled Ctrl-C is not an error.
			if err := rt.CancelRun(context.Background(), started.RunID); err != nil {
				t.Fatalf("second CancelRun: %v", err)
			}
		}
	}

	last, ok := events[len(events)-1].(client.RunFinished)
	if !ok {
		t.Fatalf("stream ends with %T, want client.RunFinished", events[len(events)-1])
	}
	if last.Outcome.Status != client.OutcomeCanceled {
		t.Fatalf("outcome = %s, want canceled", last.Outcome.Status)
	}
}

func TestCancelUnknownRun(t *testing.T) {
	rt, _ := instant(t)
	if err := rt.CancelRun(context.Background(), "run_nope"); !errors.Is(err, client.ErrRunNotFound) {
		t.Fatalf("err = %v, want ErrRunNotFound", err)
	}
}

func TestBreakingOutOfAStreamDoesNotWedgeTheMock(t *testing.T) {
	rt, session := instant(t)
	stream, _ := rt.StartRun(context.Background(), client.StartRun{SessionID: session, Prompt: "why?"})

	// Abandon the stream immediately. A pull iterator's generator must unwind on
	// its own; if it did not, the next call would block or panic.
	for range stream {
		break
	}
	if _, err := rt.ListSessions(context.Background()); err != nil {
		t.Fatalf("ListSessions after abandoning a stream: %v", err)
	}
}

func TestScriptedDelaysAreHonouredWhenNotInstant(t *testing.T) {
	rt := New()
	sessions, _ := rt.ListSessions(context.Background())
	stream, _ := rt.StartRun(context.Background(), client.StartRun{SessionID: sessions[0].ID, Prompt: "why?"})

	start := time.Now()
	for ev := range stream {
		if _, ok := ev.(client.BlockStarted); ok {
			break
		}
	}
	if elapsed := time.Since(start); elapsed < beat {
		t.Fatalf("reached the first block in %v, want at least one beat (%v)", elapsed, beat)
	}
}

func TestCreateSessionAppearsInTheList(t *testing.T) {
	rt, _ := instant(t)
	created, err := rt.CreateSession(context.Background(), client.NewSession{Title: "New", Workspace: "/tmp/ws"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created session has no id")
	}
	list, _ := rt.ListSessions(context.Background())
	for _, s := range list {
		if s.ID == created.ID {
			return
		}
	}
	t.Fatalf("created session %s is missing from the list", created.ID)
}

func TestListSessionsIsNewestFirst(t *testing.T) {
	rt, _ := instant(t)
	list, err := rt.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].UpdatedAt.Before(list[i].UpdatedAt) {
			t.Fatalf("session %d is older than %d", i-1, i)
		}
	}
}
