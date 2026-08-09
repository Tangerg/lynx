package mock

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func newSession(t *testing.T, runtime *Runtime) client.Session {
	t.Helper()
	session, err := runtime.CreateSession(t.Context(), client.NewSession{Title: "Test", Workspace: "/tmp/mock-test"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return session
}

func followAll(t *testing.T, runtime *Runtime, runID string, after client.Cursor) ([]client.Envelope, error) {
	t.Helper()
	stream, err := runtime.FollowRun(t.Context(), client.FollowRun{RunID: runID, After: after})
	if err != nil {
		return nil, err
	}
	var events []client.Envelope
	for envelope, streamErr := range stream {
		if streamErr != nil {
			return events, streamErr
		}
		events = append(events, envelope)
	}
	return events, nil
}

func TestSessionCatalogPagesSearchesAndSorts(t *testing.T) {
	runtime := New()
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		if _, err := runtime.CreateSession(t.Context(), client.NewSession{Title: title, Workspace: "/tmp/catalog"}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := runtime.ListSessions(t.Context(), client.SessionQuery{Workspace: "/tmp/catalog", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := runtime.ListSessions(t.Context(), client.SessionQuery{Workspace: "/tmp/catalog", Cursor: first.NextCursor, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
	search, err := runtime.ListSessions(t.Context(), client.SessionQuery{Search: "beta"})
	if err != nil || len(search.Items) != 1 || search.Items[0].Title != "Beta" {
		t.Fatalf("search = %+v, %v", search, err)
	}
	if _, err := runtime.ListSessions(t.Context(), client.SessionQuery{Cursor: "bogus"}); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
}

func TestSessionLifecycleHonorsRevisionAndForkCursor(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	updated, err := runtime.UpdateSession(t.Context(), client.UpdateSession{SessionID: session.ID, Title: "Renamed", Revision: session.Revision})
	if err != nil || updated.Title != "Renamed" {
		t.Fatalf("UpdateSession = %+v, %v", updated, err)
	}
	if _, err := runtime.UpdateSession(t.Context(), client.UpdateSession{SessionID: session.ID, Title: "Stale", Revision: session.Revision}); !errors.Is(err, client.ErrRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}

	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "why?"}})
	if err != nil {
		t.Fatal(err)
	}
	events, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("run produced %d events", len(events))
	}
	fork, err := runtime.ForkSession(t.Context(), client.ForkSession{SessionID: session.ID, At: events[1].Cursor, Title: "Forked"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if client.Cursor(len(snapshot.Events)) != events[1].Cursor || snapshot.Session.Title != "Forked" {
		t.Fatalf("fork snapshot = %+v", snapshot)
	}
	for _, envelope := range snapshot.Events {
		if envelope.SessionID != fork.ID {
			t.Fatalf("fork event retained source session id: %+v", envelope)
		}
	}
	if err := runtime.DeleteSession(t.Context(), client.DeleteSession{SessionID: fork.ID, Revision: fork.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetSession(t.Context(), fork.ID); !errors.Is(err, client.ErrSessionNotFound) {
		t.Fatalf("deleted session error = %v", err)
	}
}

func TestRunReplaysAndResumesApprovalWithoutDuplicates(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{
		SessionID: session.ID,
		Message:   client.Message{Text: "why?"},
		Options:   client.RunOptions{Model: "mock-balanced", Mode: client.ModeBuild, Permission: client.PermissionAsk},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("initial subscription was empty")
	}
	interrupted, ok := first[len(first)-1].Event.(client.RunInterrupted)
	if !ok {
		t.Fatalf("last event = %T, want RunInterrupted", first[len(first)-1].Event)
	}
	approval := interrupted.Interaction.(client.Approval)

	// Replaying from the cursor before the last event returns that exact event.
	replay, err := followAll(t, runtime, run.ID, first[len(first)-2].Cursor)
	if err != nil || len(replay) != 1 || replay[0].ID != first[len(first)-1].ID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: run.ID, InterruptID: approval.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberSession},
	}); err != nil {
		t.Fatal(err)
	}
	second, err := followAll(t, runtime, run.ID, first[len(first)-1].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) == 0 {
		t.Fatal("continuation was empty")
	}
	if _, ok := second[0].Event.(client.RunResumed); !ok {
		t.Fatalf("first continuation event = %T, want RunResumed", second[0].Event)
	}
	if finished, ok := second[len(second)-1].Event.(client.RunFinished); !ok || finished.Outcome.Status != client.OutcomeCompleted {
		t.Fatalf("last continuation event = %+v", second[len(second)-1].Event)
	}
	seen := make(map[string]bool)
	for _, envelope := range slices.Concat(first, second) {
		if seen[envelope.ID] {
			t.Fatalf("duplicate event id %s", envelope.ID)
		}
		seen[envelope.ID] = true
	}
}

func TestStartRunIsIdempotentForARequestIdentity(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	start := client.StartRun{
		RequestID: "req_retry_1",
		SessionID: session.ID,
		Message:   client.Message{Text: "retry this turn"},
		Options:   client.RunOptions{Mode: client.ModeBuild, Permission: client.PermissionAsk},
	}
	first, err := runtime.StartRun(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.StartRun(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("replayed start = %+v, want %+v", second, first)
	}

	conflict := start
	conflict.Message.Text = "a different turn"
	if _, err := runtime.StartRun(t.Context(), conflict); !errors.Is(err, client.ErrRequestConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
	if _, err := runtime.StartRun(t.Context(), client.StartRun{
		RequestID: "bad request", SessionID: session.ID, Message: client.Message{Text: "invalid"},
	}); err == nil {
		t.Fatal("request identity containing whitespace was accepted")
	}

	events, err := followAll(t, runtime, first.ID, first.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	started := 0
	for _, envelope := range events {
		if _, ok := envelope.Event.(client.RunStarted); ok {
			started++
		}
	}
	if started != 1 {
		t.Fatalf("idempotent start emitted %d RunStarted events", started)
	}
}

func TestResumeRunIsIdempotentForTheSameAnswer(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "why?"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	approval := before[len(before)-1].Event.(client.RunInterrupted).Interaction.(client.Approval)
	resume := client.ResumeRun{
		RunID: run.ID, InterruptID: approval.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberNone},
	}
	if err := runtime.ResumeRun(t.Context(), resume); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ResumeRun(t.Context(), resume); err != nil {
		t.Fatalf("replayed answer: %v", err)
	}
	resume.Answer = client.ApprovalAnswer{Decision: client.ApprovalDeny, Remember: client.RememberNone}
	if err := runtime.ResumeRun(t.Context(), resume); !errors.Is(err, client.ErrRequestConflict) {
		t.Fatalf("conflicting answer error = %v", err)
	}
	continued, err := followAll(t, runtime, run.ID, before[len(before)-1].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	resumed := 0
	for _, envelope := range continued {
		if _, ok := envelope.Event.(client.RunResumed); ok {
			resumed++
		}
	}
	if resumed != 1 {
		t.Fatalf("idempotent answer emitted %d RunResumed events", resumed)
	}
}

func TestFollowRunInjectsTransportFaultsWithoutChangingDurableLog(t *testing.T) {
	tests := []struct {
		name  string
		fault SubscriptionFault
		check func(*testing.T, []client.Envelope, error)
	}{
		{
			name: "disconnect", fault: SubscriptionFault{Kind: FaultDisconnect, After: 1},
			check: func(t *testing.T, events []client.Envelope, err error) {
				if len(events) != 1 || !errors.Is(err, client.ErrDisconnected) {
					t.Fatalf("events = %d, error = %v", len(events), err)
				}
			},
		},
		{
			name: "duplicate", fault: SubscriptionFault{Kind: FaultDuplicate, After: 1},
			check: func(t *testing.T, events []client.Envelope, err error) {
				if err != nil || len(events) < 2 || events[0].ID != events[1].ID {
					t.Fatalf("events = %+v, error = %v", events, err)
				}
			},
		},
		{
			name: "gap", fault: SubscriptionFault{Kind: FaultGap, After: 1},
			check: func(t *testing.T, events []client.Envelope, err error) {
				if err != nil || len(events) == 0 || events[0].Cursor < 2 {
					t.Fatalf("events = %+v, error = %v", events, err)
				}
			},
		},
		{
			name: "conflict", fault: SubscriptionFault{Kind: FaultConflict, After: 1},
			check: func(t *testing.T, events []client.Envelope, err error) {
				if err != nil || len(events) != 2 || events[0].Cursor != events[1].Cursor || events[0].ID == events[1].ID {
					t.Fatalf("events = %+v, error = %v", events, err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := New()
			runtime.Instant = true
			runtime.Faults = []SubscriptionFault{tt.fault}
			session := newSession(t, runtime)
			run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "fault"}})
			if err != nil {
				t.Fatal(err)
			}
			events, streamErr := followAll(t, runtime, run.ID, run.StartedAfter)
			tt.check(t, events, streamErr)

			replayed, err := followAll(t, runtime, run.ID, run.StartedAfter)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := runtime.GetSession(t.Context(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(replayed) != len(snapshot.Events) {
				t.Fatalf("durable replay = %d events, snapshot = %d", len(replayed), len(snapshot.Events))
			}
		})
	}
}

func TestQuestionRequiresTypedCompleteAnswer(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	runtime.Script = func(string) Script {
		return Script{
			Interaction: client.Question{InterruptID: "question_1", Title: "Choose", Fields: []client.QuestionField{{ID: "strategy", Label: "Strategy", Kind: client.QuestionSingle, Required: true, Options: []client.QuestionOption{{Value: "safe", Label: "Safe"}}}}},
			Continue: func(client.Answer) []Step {
				return []Step{{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}
			},
		}
	}
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	question := first[len(first)-1].Event.(client.RunInterrupted).Interaction.(client.Question)
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{RunID: run.ID, InterruptID: question.InterruptID, Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow}}); err == nil {
		t.Fatal("approval answer was accepted for a question")
	}
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{RunID: run.ID, InterruptID: question.InterruptID, Answer: client.QuestionAnswer{}}); err == nil {
		t.Fatal("missing required answer was accepted")
	}
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: run.ID, InterruptID: question.InterruptID,
		Answer: client.QuestionAnswer{Values: map[string][]string{"strategy": {"safe"}}},
	}); err != nil {
		t.Fatalf("complete answer: %v", err)
	}
}

func TestCancelWaitingRunFinishesIt(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "why?"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelRun(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	continued, err := followAll(t, runtime, run.ID, first[len(first)-1].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued) != 1 {
		t.Fatalf("cancel continuation = %+v", continued)
	}
	finished := continued[0].Event.(client.RunFinished)
	if finished.Outcome.Status != client.OutcomeCanceled {
		t.Fatalf("cancel outcome = %+v", finished.Outcome)
	}
}

func TestRememberedApprovalRuleSkipsMatchingInterruptAndCanBeForgotten(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	firstRun, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "first"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := followAll(t, runtime, firstRun.ID, firstRun.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	approval := first[len(first)-1].Event.(client.RunInterrupted).Interaction.(client.Approval)
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: firstRun.ID, InterruptID: approval.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberSession},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := followAll(t, runtime, firstRun.ID, first[len(first)-1].Cursor); err != nil {
		t.Fatal(err)
	}

	rules, err := runtime.ListApprovalRules(t.Context())
	if err != nil || len(rules) != 1 || rules[0].Scope != client.RememberSession {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
	secondRun, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "second"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := followAll(t, runtime, secondRun.ID, secondRun.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(second, func(envelope client.Envelope) bool {
		_, interrupted := envelope.Event.(client.RunInterrupted)
		return interrupted
	}) {
		t.Fatalf("remembered rule did not skip approval: %+v", second)
	}
	if _, ok := second[len(second)-1].Event.(client.RunFinished); !ok {
		t.Fatalf("second run did not finish: %+v", second)
	}
	if err := runtime.DeleteApprovalRule(t.Context(), rules[0].ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := runtime.ListApprovalRules(t.Context()); len(rules) != 0 {
		t.Fatalf("deleted rule remains: %+v", rules)
	}
}

func TestCanceledSubscriptionDoesNotCancelLogicalRun(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{Delay: 100 * time.Millisecond, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}}
	}
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "wait"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream, err := runtime.FollowRun(ctx, client.FollowRun{RunID: run.ID, After: run.StartedAfter})
	if err != nil {
		t.Fatal(err)
	}
	for envelope, streamErr := range stream {
		if streamErr != nil {
			break
		}
		if _, ok := envelope.Event.(client.BlockCompleted); ok {
			cancel()
		}
	}
	replayed, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replayed[len(replayed)-1].Event.(client.RunFinished); !ok {
		t.Fatalf("logical run did not finish after subscriber left: %+v", replayed)
	}
}

func TestFollowRunRejectsCursorOutsideSessionWithoutOverflow(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), client.StartRun{
		SessionID: session.ID,
		Message:   client.Message{Text: "finish"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.FollowRun(t.Context(), client.FollowRun{RunID: run.ID, After: ^client.Cursor(0)}); err == nil {
		t.Fatal("FollowRun accepted a cursor beyond the session")
	}
}

func TestRuntimeSupportsConcurrentCatalogReadsDuringStreaming(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	var wait sync.WaitGroup
	for range 16 {
		wait.Go(func() {
			if _, err := runtime.ListSessions(t.Context(), client.SessionQuery{}); err != nil {
				t.Errorf("ListSessions: %v", err)
			}
		})
	}
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "why?"}})
	if err != nil {
		t.Fatal(err)
	}
	wait.Go(func() {
		if _, err := followAll(t, runtime, run.ID, run.StartedAfter); err != nil {
			t.Errorf("FollowRun: %v", err)
		}
	})
	wait.Wait()
}

func TestStartRunValidatesAndCopiesAttachments(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	invalid := client.Attachment{Kind: client.AttachmentText, Name: "broken.txt", Path: "/tmp/broken.txt"}
	if _, err := runtime.StartRun(t.Context(), client.StartRun{
		SessionID: session.ID, Message: client.Message{Attachments: []client.Attachment{invalid}},
	}); err == nil {
		t.Fatal("invalid attachment was accepted")
	}

	attachments := []client.Attachment{{
		ID: "att_1", Kind: client.AttachmentText, Name: "notes.txt",
		Path: "/tmp/notes.txt", MimeType: "text/plain", Size: 5,
	}}
	run, err := runtime.StartRun(t.Context(), client.StartRun{
		SessionID: session.ID, Message: client.Message{Text: "inspect", Attachments: attachments},
	})
	if err != nil {
		t.Fatal(err)
	}
	attachments[0].Name = "mutated.txt"
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got client.Block
	for _, envelope := range snapshot.Events {
		if event, ok := envelope.Event.(client.BlockCompleted); ok && envelope.RunID == run.ID && event.Block.Kind == client.BlockUser {
			got = event.Block
		}
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Name != "notes.txt" {
		t.Fatalf("stored attachments = %+v", got.Attachments)
	}
}
