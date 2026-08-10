package mock

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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

func mustStartRun(t *testing.T, runtime *Runtime, input client.StartRun) client.Run {
	t.Helper()
	run, err := runtime.StartRun(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func mustFollowAll(t *testing.T, runtime *Runtime, runID string, after client.Cursor) []client.Envelope {
	t.Helper()
	events, err := followAll(t, runtime, runID, after)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func mustSnapshot(t *testing.T, runtime *Runtime, sessionID string) client.SessionSnapshot {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestSessionCatalogPagesSearchesAndSorts(t *testing.T) {
	runtime := New()
	seedCatalog(t, runtime, "Alpha", "Beta", "Gamma")
	first := mustListSessions(t, runtime, client.SessionQuery{Workspace: "/tmp/catalog", Limit: 2})
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := mustListSessions(t, runtime, client.SessionQuery{Workspace: "/tmp/catalog", Cursor: first.NextCursor, Limit: 2})
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
	search := mustListSessions(t, runtime, client.SessionQuery{Search: "beta"})
	if len(search.Items) != 1 || search.Items[0].Title != "Beta" {
		t.Fatalf("search = %+v", search)
	}
	if _, err := runtime.ListSessions(t.Context(), client.SessionQuery{Cursor: "bogus"}); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
}

func TestToolFixtureStreamsTheExactCompletedOutput(t *testing.T) {
	want := "first line\nsecond line\n"
	steps := tool("tool", client.ToolShell, "shell", "go test ./...", client.ToolOK, want, "", time.Second)
	var streamed strings.Builder
	var completed client.Block
	for _, step := range steps {
		switch event := step.Event.(type) {
		case client.BlockDelta:
			if event.BlockID != "tool" {
				t.Fatalf("delta block id = %q", event.BlockID)
			}
			streamed.WriteString(event.Text)
		case client.BlockCompleted:
			completed = event.Block
		}
	}
	if got := streamed.String(); got != want {
		t.Fatalf("streamed output = %q, want %q", got, want)
	}
	if completed.Tool == nil || completed.Tool.Output != want {
		t.Fatalf("completed tool = %+v", completed.Tool)
	}
}

func seedCatalog(t *testing.T, runtime *Runtime, titles ...string) {
	t.Helper()
	for _, title := range titles {
		if _, err := runtime.CreateSession(t.Context(), client.NewSession{Title: title, Workspace: "/tmp/catalog"}); err != nil {
			t.Fatal(err)
		}
	}
}

func mustListSessions(t *testing.T, runtime *Runtime, query client.SessionQuery) client.SessionPage {
	t.Helper()
	page, err := runtime.ListSessions(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestSynchronousRuntimeQueriesHonorCancellation(t *testing.T) {
	runtime := New()
	session := newSession(t, runtime)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{"list sessions", func(ctx context.Context) error {
			_, err := runtime.ListSessions(ctx, client.SessionQuery{})
			return err
		}},
		{"get session", func(ctx context.Context) error { _, err := runtime.GetSession(ctx, session.ID); return err }},
		{"create session", func(ctx context.Context) error {
			_, err := runtime.CreateSession(ctx, client.NewSession{Workspace: "/tmp/canceled"})
			return err
		}},
		{"update session", func(ctx context.Context) error {
			_, err := runtime.UpdateSession(ctx, client.UpdateSession{SessionID: session.ID, Title: "Canceled"})
			return err
		}},
		{"fork session", func(ctx context.Context) error {
			_, err := runtime.ForkSession(ctx, client.ForkSession{SessionID: session.ID})
			return err
		}},
		{"delete session", func(ctx context.Context) error {
			return runtime.DeleteSession(ctx, client.DeleteSession{SessionID: session.ID})
		}},
		{"list models", func(ctx context.Context) error { _, err := runtime.ListModels(ctx); return err }},
		{"list approval rules", func(ctx context.Context) error { _, err := runtime.ListApprovalRules(ctx); return err }},
		{"delete approval rule", func(ctx context.Context) error { return runtime.DeleteApprovalRule(ctx, "rule_missing") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", err)
			}
		})
	}
}

func TestSessionLifecycleHonorsRevisionAndForkCursor(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}}
	}
	session := newSession(t, runtime)
	updated, err := runtime.UpdateSession(t.Context(), client.UpdateSession{SessionID: session.ID, Title: "Renamed", Revision: session.Revision})
	if err != nil || updated.Title != "Renamed" {
		t.Fatalf("UpdateSession = %+v, %v", updated, err)
	}
	if _, err := runtime.UpdateSession(t.Context(), client.UpdateSession{SessionID: session.ID, Title: "Stale", Revision: session.Revision}); !errors.Is(err, client.ErrRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}

	run := mustStartRun(t, runtime, client.StartRun{SessionID: session.ID, Message: client.Message{Text: "why?"}})
	events := mustFollowAll(t, runtime, run.ID, run.StartedAfter)
	if len(events) < 3 {
		t.Fatalf("run produced %d events", len(events))
	}
	fork, err := runtime.ForkSession(t.Context(), client.ForkSession{SessionID: session.ID, At: events[len(events)-1].Cursor, Title: "Forked"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := mustSnapshot(t, runtime, fork.ID)
	if client.Cursor(len(snapshot.Events)) != events[len(events)-1].Cursor || snapshot.Session.Title != "Forked" {
		t.Fatalf("fork snapshot = %+v", snapshot)
	}
	requireEventSession(t, snapshot.Events, fork.ID)
	if err := runtime.DeleteSession(t.Context(), client.DeleteSession{SessionID: fork.ID, Revision: fork.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetSession(t.Context(), fork.ID); !errors.Is(err, client.ErrSessionNotFound) {
		t.Fatalf("deleted session error = %v", err)
	}
}

func requireEventSession(t *testing.T, events []client.Envelope, sessionID string) {
	t.Helper()
	for _, envelope := range events {
		if envelope.SessionID != sessionID {
			t.Fatalf("event retained source session id: %+v", envelope)
		}
	}
}

func TestRunReplaysAndResumesApprovalWithoutDuplicates(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run := mustStartRun(t, runtime, client.StartRun{
		SessionID: session.ID,
		Message:   client.Message{Text: "why?"},
		Options:   client.RunOptions{Model: "mock-balanced", Mode: client.ModeBuild, Permission: client.PermissionAsk},
	})
	first := mustFollowAll(t, runtime, run.ID, run.StartedAfter)
	approval := requireApprovalInterrupt(t, first)

	// Replaying from the cursor before the last event returns that exact event.
	requireLastEventReplay(t, runtime, run.ID, first)
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: run.ID, InterruptID: approval.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberSession},
	}); err != nil {
		t.Fatal(err)
	}
	second := mustFollowAll(t, runtime, run.ID, first[len(first)-1].Cursor)
	requireCompletedContinuation(t, second)
	requireUniqueEventIDs(t, slices.Concat(first, second))
}

func requireApprovalInterrupt(t *testing.T, events []client.Envelope) client.Approval {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("initial subscription was empty")
	}
	interrupted, ok := events[len(events)-1].Event.(client.RunInterrupted)
	if !ok {
		t.Fatalf("last event = %T, want RunInterrupted", events[len(events)-1].Event)
	}
	approval, ok := interrupted.Interaction.(client.Approval)
	if !ok {
		t.Fatalf("interaction = %T, want Approval", interrupted.Interaction)
	}
	return approval
}

func requireLastEventReplay(t *testing.T, runtime *Runtime, runID string, events []client.Envelope) {
	t.Helper()
	last := events[len(events)-1]
	replay, err := followAll(t, runtime, runID, events[len(events)-2].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].ID != last.ID {
		t.Fatalf("replay = %+v, want event %s", replay, last.ID)
	}
}

func requireCompletedContinuation(t *testing.T, events []client.Envelope) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("continuation was empty")
	}
	if _, ok := events[0].Event.(client.RunResumed); !ok {
		t.Fatalf("first continuation event = %T, want RunResumed", events[0].Event)
	}
	finished, ok := events[len(events)-1].Event.(client.RunFinished)
	if !ok {
		t.Fatalf("last continuation event = %T, want RunFinished", events[len(events)-1].Event)
	}
	if finished.Outcome.Status != client.OutcomeCompleted {
		t.Fatalf("outcome status = %s, want completed", finished.Outcome.Status)
	}
}

func requireUniqueEventIDs(t *testing.T, events []client.Envelope) {
	t.Helper()
	seen := make(map[string]bool, len(events))
	for _, envelope := range events {
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
	for _, kind := range []FaultKind{FaultDisconnect, FaultDuplicate, FaultGap, FaultConflict} {
		t.Run(string(kind), func(t *testing.T) { exerciseTransportFault(t, kind) })
	}
}

func exerciseTransportFault(t *testing.T, kind FaultKind) {
	t.Helper()
	runtime := New()
	runtime.Instant = true
	runtime.Faults = []SubscriptionFault{{Kind: kind, After: 1}}
	session := newSession(t, runtime)
	run := mustStartRun(t, runtime, client.StartRun{SessionID: session.ID, Message: client.Message{Text: "fault"}})
	events, streamErr := followAll(t, runtime, run.ID, run.StartedAfter)
	requireInjectedFault(t, kind, events, streamErr)
	replayed := mustFollowAll(t, runtime, run.ID, run.StartedAfter)
	snapshot := mustSnapshot(t, runtime, session.ID)
	if len(replayed) != len(snapshot.Events) {
		t.Fatalf("durable replay = %d events, snapshot = %d", len(replayed), len(snapshot.Events))
	}
}

func requireInjectedFault(t *testing.T, kind FaultKind, events []client.Envelope, err error) {
	t.Helper()
	switch kind {
	case FaultDisconnect:
		requireDisconnectFault(t, events, err)
	case FaultDuplicate:
		requireDuplicateFault(t, events, err)
	case FaultGap:
		requireGapFault(t, events, err)
	case FaultConflict:
		requireConflictFault(t, events, err)
	default:
		t.Fatalf("unsupported fault %q", kind)
	}
}

func requireDisconnectFault(t *testing.T, events []client.Envelope, err error) {
	t.Helper()
	if len(events) != 1 || !errors.Is(err, client.ErrDisconnected) {
		t.Fatalf("events = %d, error = %v", len(events), err)
	}
}

func requireDuplicateFault(t *testing.T, events []client.Envelope, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 || events[0].ID != events[1].ID {
		t.Fatalf("events = %+v, want a duplicate prefix", events)
	}
}

func requireGapFault(t *testing.T, events []client.Envelope, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0].Cursor < 2 {
		t.Fatalf("events = %+v, want a leading cursor gap", events)
	}
}

func requireConflictFault(t *testing.T, events []client.Envelope, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want a two-event conflict", events)
	}
	if events[0].Cursor != events[1].Cursor || events[0].ID == events[1].ID {
		t.Fatalf("events = %+v, want shared cursor with distinct ids", events)
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
	if err := runtime.CancelRun(t.Context(), client.CancelRun{RunID: run.ID}); err != nil {
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
	firstRun := mustStartRun(t, runtime, client.StartRun{SessionID: session.ID, Message: client.Message{Text: "first"}})
	first := mustFollowAll(t, runtime, firstRun.ID, firstRun.StartedAfter)
	approval := first[len(first)-1].Event.(client.RunInterrupted).Interaction.(client.Approval)
	if err := runtime.ResumeRun(t.Context(), client.ResumeRun{
		RunID: firstRun.ID, InterruptID: approval.InterruptID,
		Answer: client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberSession},
	}); err != nil {
		t.Fatal(err)
	}
	_ = mustFollowAll(t, runtime, firstRun.ID, first[len(first)-1].Cursor)

	rules, err := runtime.ListApprovalRules(t.Context())
	if err != nil || len(rules) != 1 || rules[0].Scope != client.RememberSession {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
	secondRun := mustStartRun(t, runtime, client.StartRun{SessionID: session.ID, Message: client.Message{Text: "second"}})
	second := mustFollowAll(t, runtime, secondRun.ID, secondRun.StartedAfter)
	requireFinishedWithoutInterrupt(t, second)
	if err := runtime.DeleteApprovalRule(t.Context(), rules[0].ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := runtime.ListApprovalRules(t.Context()); len(rules) != 0 {
		t.Fatalf("deleted rule remains: %+v", rules)
	}
}

func requireFinishedWithoutInterrupt(t *testing.T, events []client.Envelope) {
	t.Helper()
	if slices.ContainsFunc(events, func(envelope client.Envelope) bool {
		_, interrupted := envelope.Event.(client.RunInterrupted)
		return interrupted
	}) {
		t.Fatalf("remembered rule did not skip approval: %+v", events)
	}
	if _, ok := events[len(events)-1].Event.(client.RunFinished); !ok {
		t.Fatalf("run did not finish: %+v", events)
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

func TestConcurrentIdempotentStartsBuildOnceOutsideTheRuntimeLock(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var builds atomic.Int32
	runtime.Script = func(string) Script {
		builds.Add(1)
		if _, err := runtime.ListSessions(t.Context(), client.SessionQuery{}); err != nil {
			panic(err)
		}
		entered <- struct{}{}
		<-release
		return completedScript()
	}
	input := client.StartRun{RequestID: "req_same", SessionID: session.ID, Message: client.Message{Text: "once"}}
	type result struct {
		run client.Run
		err error
	}
	results := make(chan result, 2)
	go func() {
		run, err := runtime.StartRun(t.Context(), input)
		results <- result{run: run, err: err}
	}()
	<-entered
	go func() {
		run, err := runtime.StartRun(t.Context(), input)
		results <- result{run: run, err: err}
	}()
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.run.ID == "" || first.run.ID != second.run.ID {
		t.Fatalf("idempotent starts = %+v and %+v", first, second)
	}
	if builds.Load() != 1 {
		t.Fatalf("script built %d times", builds.Load())
	}
}

func TestCancelStartCreatesATombstoneAndPreventsLateCommit(t *testing.T) {
	runtime := New()
	session := newSession(t, runtime)
	entered := make(chan struct{})
	release := make(chan struct{})
	runtime.Script = func(string) Script {
		close(entered)
		<-release
		return completedScript()
	}
	input := client.StartRun{RequestID: "req_cancel", SessionID: session.ID, Message: client.Message{Text: "cancel"}}
	started := make(chan error, 1)
	go func() {
		_, err := runtime.StartRun(t.Context(), input)
		started <- err
	}()
	<-entered
	if err := runtime.CancelRun(t.Context(), client.CancelRun{SessionID: session.ID, RequestID: input.RequestID}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-started; !errors.Is(err, client.ErrRunCanceled) {
		t.Fatalf("late start error = %v, want ErrRunCanceled", err)
	}
	if _, err := runtime.StartRun(t.Context(), input); !errors.Is(err, client.ErrRunCanceled) {
		t.Fatalf("tombstoned retry error = %v, want ErrRunCanceled", err)
	}
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil || snapshot.Cursor != 0 {
		t.Fatalf("canceled start committed state: %+v", snapshot)
	}
}

func TestScriptBuilderPanicIsStableAcrossIdempotentRetries(t *testing.T) {
	runtime := New()
	session := newSession(t, runtime)
	var builds atomic.Int32
	runtime.Script = func(string) Script {
		builds.Add(1)
		panic("fixture exploded")
	}
	input := client.StartRun{RequestID: "req_panic", SessionID: session.ID, Message: client.Message{Text: "panic"}}
	_, first := runtime.StartRun(t.Context(), input)
	_, second := runtime.StartRun(t.Context(), input)
	if first == nil || second == nil || first.Error() != second.Error() || !strings.Contains(first.Error(), "fixture exploded") {
		t.Fatalf("panic errors = %v and %v", first, second)
	}
	if builds.Load() != 1 {
		t.Fatalf("panicking builder ran %d times", builds.Load())
	}
}

func TestCancelRunFinishesImmediatelyWithoutPostFinishEvents(t *testing.T) {
	runtime := New()
	session := newSession(t, runtime)
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{Delay: time.Hour, Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}}
	}
	run, err := runtime.StartRun(t.Context(), client.StartRun{RequestID: "req_active", SessionID: session.ID, Message: client.Message{Text: "wait"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelRun(t.Context(), client.CancelRun{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	events, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	finished, ok := events[len(events)-1].Event.(client.RunFinished)
	if !ok || finished.Outcome.Status != client.OutcomeCanceled {
		t.Fatalf("last event = %+v, want canceled finish", events[len(events)-1])
	}
	if err := runtime.CancelRun(t.Context(), client.CancelRun{RunID: run.ID}); err != nil {
		t.Fatalf("idempotent cancellation: %v", err)
	}
}

func TestContinuationRunsOutsideLockAndCancellationWinsItsCommitRace(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	entered := make(chan struct{})
	release := make(chan struct{})
	runtime.Script = func(string) Script {
		return Script{
			Interaction: client.Question{InterruptID: "question_1", Title: "Choose", Fields: []client.QuestionField{{ID: "name", Label: "Name", Kind: client.QuestionText, Required: true}}},
			Continue: func(client.Answer) []Step {
				if _, err := runtime.ListSessions(t.Context(), client.SessionQuery{}); err != nil {
					panic(err)
				}
				close(entered)
				<-release
				return completedScript().Prelude
			},
		}
	}
	run, err := runtime.StartRun(t.Context(), client.StartRun{SessionID: session.ID, Message: client.Message{Text: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	events, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := events[len(events)-1].Event.(client.RunInterrupted)
	resumeErr := make(chan error, 1)
	go func() {
		resumeErr <- runtime.ResumeRun(t.Context(), client.ResumeRun{
			RunID: run.ID, InterruptID: client.InteractionID(interrupted.Interaction),
			Answer: client.QuestionAnswer{Values: map[string][]string{"name": {"cache"}}},
		})
	}()
	<-entered
	if err := runtime.CancelRun(t.Context(), client.CancelRun{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-resumeErr; !errors.Is(err, client.ErrRunCanceled) {
		t.Fatalf("resume error = %v, want ErrRunCanceled", err)
	}
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Active != nil {
		t.Fatalf("canceled resumption left active run: %+v", snapshot.Active)
	}
}

func completedScript() Script {
	return Script{Prelude: []Step{{Event: client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCompleted}}}}}
}
