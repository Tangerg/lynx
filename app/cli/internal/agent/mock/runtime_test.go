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

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func newSession(t *testing.T, runtime *Runtime) agent.Session {
	t.Helper()
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: "Test", Workspace: "/tmp/mock-test"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return session
}

func followAll(t *testing.T, runtime *Runtime, runID string, after agent.Cursor) ([]agent.Envelope, error) {
	t.Helper()
	stream, err := runtime.FollowRun(t.Context(), agent.FollowRun{RunID: runID, After: after})
	if err != nil {
		return nil, err
	}
	var events []agent.Envelope
	for envelope, streamErr := range stream {
		if streamErr != nil {
			return events, streamErr
		}
		events = append(events, envelope)
	}
	return events, nil
}

func mustStartRun(t *testing.T, runtime *Runtime, input agent.StartRun) agent.Run {
	t.Helper()
	run, err := runtime.StartRun(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func mustFollowAll(t *testing.T, runtime *Runtime, runID string, after agent.Cursor) []agent.Envelope {
	t.Helper()
	events, err := followAll(t, runtime, runID, after)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func mustSnapshot(t *testing.T, runtime *Runtime, sessionID string) agent.SessionSnapshot {
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
	first := mustListSessions(t, runtime, agent.SessionQuery{Workspace: "/tmp/catalog", Limit: 2})
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	second := mustListSessions(t, runtime, agent.SessionQuery{Workspace: "/tmp/catalog", Cursor: first.NextCursor, Limit: 2})
	if len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
	search := mustListSessions(t, runtime, agent.SessionQuery{Search: "beta"})
	if len(search.Items) != 1 || search.Items[0].Title != "Beta" {
		t.Fatalf("search = %+v", search)
	}
	if _, err := runtime.ListSessions(t.Context(), agent.SessionQuery{Cursor: "bogus"}); err == nil {
		t.Fatal("invalid cursor was accepted")
	}
}

func TestToolFixtureStreamsTheExactCompletedOutput(t *testing.T) {
	want := "first line\nsecond line\n"
	steps := tool("tool", agent.ToolShell, "shell", "go test ./...", agent.ToolOK, want, "", time.Second)
	var streamed strings.Builder
	var completed agent.Block
	for _, step := range steps {
		switch event := step.Event.(type) {
		case agent.BlockDelta:
			if event.BlockID != "tool" {
				t.Fatalf("delta block id = %q", event.BlockID)
			}
			streamed.WriteString(event.Text)
		case agent.BlockCompleted:
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
		if _, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: title, Workspace: "/tmp/catalog"}); err != nil {
			t.Fatal(err)
		}
	}
}

func mustListSessions(t *testing.T, runtime *Runtime, query agent.SessionQuery) agent.SessionPage {
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
			_, err := runtime.ListSessions(ctx, agent.SessionQuery{})
			return err
		}},
		{"get session", func(ctx context.Context) error { _, err := runtime.GetSession(ctx, session.ID); return err }},
		{"create session", func(ctx context.Context) error {
			_, err := runtime.CreateSession(ctx, agent.CreateSession{Workspace: "/tmp/canceled"})
			return err
		}},
		{"update session", func(ctx context.Context) error {
			_, err := runtime.UpdateSession(ctx, agent.UpdateSession{SessionID: session.ID, Title: "Canceled"})
			return err
		}},
		{"fork session", func(ctx context.Context) error {
			_, err := runtime.ForkSession(ctx, agent.ForkSession{SessionID: session.ID})
			return err
		}},
		{"delete session", func(ctx context.Context) error {
			return runtime.DeleteSession(ctx, agent.DeleteSession{SessionID: session.ID})
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
		return Script{Prelude: []Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session := newSession(t, runtime)
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{SessionID: session.ID, Title: "Renamed", Revision: session.Revision})
	if err != nil || updated.Title != "Renamed" {
		t.Fatalf("UpdateSession = %+v, %v", updated, err)
	}
	if _, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{SessionID: session.ID, Title: "Stale", Revision: session.Revision}); !errors.Is(err, agent.ErrRevisionConflict) {
		t.Fatalf("stale update error = %v", err)
	}

	run := mustStartRun(t, runtime, agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "why?"}})
	events := mustFollowAll(t, runtime, run.ID, run.StartedAfter)
	if len(events) < 3 {
		t.Fatalf("run produced %d events", len(events))
	}
	fork, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: session.ID, At: events[len(events)-1].Cursor, Title: "Forked"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := mustSnapshot(t, runtime, fork.ID)
	if agent.Cursor(len(snapshot.Events)) != events[len(events)-1].Cursor || snapshot.Session.Title != "Forked" {
		t.Fatalf("fork snapshot = %+v", snapshot)
	}
	requireEventSession(t, snapshot.Events, fork.ID)
	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: fork.ID, Revision: fork.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetSession(t.Context(), fork.ID); !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("deleted session error = %v", err)
	}
}

func requireEventSession(t *testing.T, events []agent.Envelope, sessionID string) {
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
	run := mustStartRun(t, runtime, agent.StartRun{
		SessionID: session.ID,
		Message:   agent.Message{Text: "why?"},
		Options:   agent.RunOptions{Model: "mock-balanced", Mode: agent.ModeBuild, Permission: agent.PermissionAsk},
	})
	first := mustFollowAll(t, runtime, run.ID, run.StartedAfter)
	approval := requireApprovalInterrupt(t, first)

	// Replaying from the cursor before the last event returns that exact event.
	requireLastEventReplay(t, runtime, run.ID, first)
	if err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: run.ID, InterruptID: approval.InterruptID,
		Answer: agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberSession},
	}); err != nil {
		t.Fatal(err)
	}
	second := mustFollowAll(t, runtime, run.ID, first[len(first)-1].Cursor)
	requireCompletedContinuation(t, second)
	requireUniqueEventIDs(t, slices.Concat(first, second))
}

func requireApprovalInterrupt(t *testing.T, events []agent.Envelope) agent.Approval {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("initial subscription was empty")
	}
	interrupted, ok := events[len(events)-1].Event.(agent.RunInterrupted)
	if !ok {
		t.Fatalf("last event = %T, want RunInterrupted", events[len(events)-1].Event)
	}
	approval, ok := interrupted.Interaction.(agent.Approval)
	if !ok {
		t.Fatalf("interaction = %T, want Approval", interrupted.Interaction)
	}
	return approval
}

func requireLastEventReplay(t *testing.T, runtime *Runtime, runID string, events []agent.Envelope) {
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

func requireCompletedContinuation(t *testing.T, events []agent.Envelope) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("continuation was empty")
	}
	if _, ok := events[0].Event.(agent.RunResumed); !ok {
		t.Fatalf("first continuation event = %T, want RunResumed", events[0].Event)
	}
	finished, ok := events[len(events)-1].Event.(agent.RunFinished)
	if !ok {
		t.Fatalf("last continuation event = %T, want RunFinished", events[len(events)-1].Event)
	}
	if finished.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("outcome status = %s, want completed", finished.Outcome.Status)
	}
}

func requireUniqueEventIDs(t *testing.T, events []agent.Envelope) {
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
	start := agent.StartRun{
		RequestID: "req_retry_1",
		SessionID: session.ID,
		Message:   agent.Message{Text: "retry this turn"},
		Options:   agent.RunOptions{Mode: agent.ModeBuild, Permission: agent.PermissionAsk},
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
	if _, err := runtime.StartRun(t.Context(), conflict); !errors.Is(err, agent.ErrRequestConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
	if _, err := runtime.StartRun(t.Context(), agent.StartRun{
		RequestID: "bad request", SessionID: session.ID, Message: agent.Message{Text: "invalid"},
	}); err == nil {
		t.Fatal("request identity containing whitespace was accepted")
	}

	events, err := followAll(t, runtime, first.ID, first.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	started := 0
	for _, envelope := range events {
		if _, ok := envelope.Event.(agent.RunStarted); ok {
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
	run, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "why?"}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	approval := before[len(before)-1].Event.(agent.RunInterrupted).Interaction.(agent.Approval)
	resume := agent.ResumeRun{
		RunID: run.ID, InterruptID: approval.InterruptID,
		Answer: agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberNone},
	}
	if err := runtime.ResumeRun(t.Context(), resume); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ResumeRun(t.Context(), resume); err != nil {
		t.Fatalf("replayed answer: %v", err)
	}
	resume.Answer = agent.ApprovalAnswer{Decision: agent.ApprovalDeny, Remember: agent.RememberNone}
	if err := runtime.ResumeRun(t.Context(), resume); !errors.Is(err, agent.ErrRequestConflict) {
		t.Fatalf("conflicting answer error = %v", err)
	}
	continued, err := followAll(t, runtime, run.ID, before[len(before)-1].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	resumed := 0
	for _, envelope := range continued {
		if _, ok := envelope.Event.(agent.RunResumed); ok {
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
	run := mustStartRun(t, runtime, agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "fault"}})
	events, streamErr := followAll(t, runtime, run.ID, run.StartedAfter)
	requireInjectedFault(t, kind, events, streamErr)
	replayed := mustFollowAll(t, runtime, run.ID, run.StartedAfter)
	snapshot := mustSnapshot(t, runtime, session.ID)
	if len(replayed) != len(snapshot.Events) {
		t.Fatalf("durable replay = %d events, snapshot = %d", len(replayed), len(snapshot.Events))
	}
}

func requireInjectedFault(t *testing.T, kind FaultKind, events []agent.Envelope, err error) {
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

func requireDisconnectFault(t *testing.T, events []agent.Envelope, err error) {
	t.Helper()
	if len(events) != 1 || !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("events = %d, error = %v", len(events), err)
	}
}

func requireDuplicateFault(t *testing.T, events []agent.Envelope, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 || events[0].ID != events[1].ID {
		t.Fatalf("events = %+v, want a duplicate prefix", events)
	}
}

func requireGapFault(t *testing.T, events []agent.Envelope, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0].Cursor < 2 {
		t.Fatalf("events = %+v, want a leading cursor gap", events)
	}
}

func requireConflictFault(t *testing.T, events []agent.Envelope, err error) {
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
			Interaction: agent.Question{InterruptID: "question_1", Title: "Choose", Fields: []agent.QuestionField{{ID: "strategy", Label: "Strategy", Kind: agent.QuestionSingle, Required: true, Options: []agent.QuestionOption{{Value: "safe", Label: "Safe"}}}}},
			Continue: func(agent.Answer) []Step {
				return []Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	question := first[len(first)-1].Event.(agent.RunInterrupted).Interaction.(agent.Question)
	if err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: run.ID, InterruptID: question.InterruptID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalAllow}}); err == nil {
		t.Fatal("approval answer was accepted for a question")
	}
	if err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: run.ID, InterruptID: question.InterruptID, Answer: agent.QuestionAnswer{}}); err == nil {
		t.Fatal("missing required answer was accepted")
	}
	if err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: run.ID, InterruptID: question.InterruptID,
		Answer: agent.QuestionAnswer{Values: map[string][]string{"strategy": {"safe"}}},
	}); err != nil {
		t.Fatalf("complete answer: %v", err)
	}
}

func TestCancelWaitingRunFinishesIt(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "why?"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	continued, err := followAll(t, runtime, run.ID, first[len(first)-1].Cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued) != 1 {
		t.Fatalf("cancel continuation = %+v", continued)
	}
	finished := continued[0].Event.(agent.RunFinished)
	if finished.Outcome.Status != agent.OutcomeCanceled {
		t.Fatalf("cancel outcome = %+v", finished.Outcome)
	}
}

func TestRememberedApprovalRuleSkipsMatchingInterruptAndCanBeForgotten(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	firstRun := mustStartRun(t, runtime, agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "first"}})
	first := mustFollowAll(t, runtime, firstRun.ID, firstRun.StartedAfter)
	approval := first[len(first)-1].Event.(agent.RunInterrupted).Interaction.(agent.Approval)
	if err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: firstRun.ID, InterruptID: approval.InterruptID,
		Answer: agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberSession},
	}); err != nil {
		t.Fatal(err)
	}
	_ = mustFollowAll(t, runtime, firstRun.ID, first[len(first)-1].Cursor)

	rules, err := runtime.ListApprovalRules(t.Context())
	if err != nil || len(rules) != 1 || rules[0].Scope != agent.RememberSession {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
	secondRun := mustStartRun(t, runtime, agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "second"}})
	second := mustFollowAll(t, runtime, secondRun.ID, secondRun.StartedAfter)
	requireFinishedWithoutInterrupt(t, second)
	if err := runtime.DeleteApprovalRule(t.Context(), rules[0].ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := runtime.ListApprovalRules(t.Context()); len(rules) != 0 {
		t.Fatalf("deleted rule remains: %+v", rules)
	}
}

func requireFinishedWithoutInterrupt(t *testing.T, events []agent.Envelope) {
	t.Helper()
	if slices.ContainsFunc(events, func(envelope agent.Envelope) bool {
		_, interrupted := envelope.Event.(agent.RunInterrupted)
		return interrupted
	}) {
		t.Fatalf("remembered rule did not skip approval: %+v", events)
	}
	if _, ok := events[len(events)-1].Event.(agent.RunFinished); !ok {
		t.Fatalf("run did not finish: %+v", events)
	}
}

func TestCanceledSubscriptionDoesNotCancelLogicalRun(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{Delay: 100 * time.Millisecond, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "wait"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stream, err := runtime.FollowRun(ctx, agent.FollowRun{RunID: run.ID, After: run.StartedAfter})
	if err != nil {
		t.Fatal(err)
	}
	for envelope, streamErr := range stream {
		if streamErr != nil {
			break
		}
		if _, ok := envelope.Event.(agent.BlockCompleted); ok {
			cancel()
		}
	}
	replayed, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := replayed[len(replayed)-1].Event.(agent.RunFinished); !ok {
		t.Fatalf("logical run did not finish after subscriber left: %+v", replayed)
	}
}

func TestFollowRunRejectsCursorOutsideSessionWithoutOverflow(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session := newSession(t, runtime)
	run, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: session.ID,
		Message:   agent.Message{Text: "finish"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.FollowRun(t.Context(), agent.FollowRun{RunID: run.ID, After: ^agent.Cursor(0)}); err == nil {
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
			if _, err := runtime.ListSessions(t.Context(), agent.SessionQuery{}); err != nil {
				t.Errorf("ListSessions: %v", err)
			}
		})
	}
	run, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "why?"}})
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
	invalid := agent.Attachment{Kind: agent.AttachmentText, Name: "broken.txt", Path: "/tmp/broken.txt"}
	if _, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: session.ID, Message: agent.Message{Attachments: []agent.Attachment{invalid}},
	}); err == nil {
		t.Fatal("invalid attachment was accepted")
	}

	attachments := []agent.Attachment{{
		ID: "att_1", Kind: agent.AttachmentText, Name: "notes.txt",
		Path: "/tmp/notes.txt", MimeType: "text/plain", Size: 5,
	}}
	run, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: session.ID, Message: agent.Message{Text: "inspect", Attachments: attachments},
	})
	if err != nil {
		t.Fatal(err)
	}
	attachments[0].Name = "mutated.txt"
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got agent.Block
	for _, envelope := range snapshot.Events {
		if event, ok := envelope.Event.(agent.BlockCompleted); ok && envelope.RunID == run.ID && event.Block.Kind == agent.BlockUser {
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
		if _, err := runtime.ListSessions(t.Context(), agent.SessionQuery{}); err != nil {
			panic(err)
		}
		entered <- struct{}{}
		<-release
		return completedScript()
	}
	input := agent.StartRun{RequestID: "req_same", SessionID: session.ID, Message: agent.Message{Text: "once"}}
	type result struct {
		run agent.Run
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
	input := agent.StartRun{RequestID: "req_cancel", SessionID: session.ID, Message: agent.Message{Text: "cancel"}}
	started := make(chan error, 1)
	go func() {
		_, err := runtime.StartRun(t.Context(), input)
		started <- err
	}()
	<-entered
	if err := runtime.CancelRun(t.Context(), agent.CancelRun{SessionID: session.ID, RequestID: input.RequestID}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-started; !errors.Is(err, agent.ErrRunCanceled) {
		t.Fatalf("late start error = %v, want ErrRunCanceled", err)
	}
	if _, err := runtime.StartRun(t.Context(), input); !errors.Is(err, agent.ErrRunCanceled) {
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
	input := agent.StartRun{RequestID: "req_panic", SessionID: session.ID, Message: agent.Message{Text: "panic"}}
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
		return Script{Prelude: []Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	run, err := runtime.StartRun(t.Context(), agent.StartRun{RequestID: "req_active", SessionID: session.ID, Message: agent.Message{Text: "wait"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	events, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	finished, ok := events[len(events)-1].Event.(agent.RunFinished)
	if !ok || finished.Outcome.Status != agent.OutcomeCanceled {
		t.Fatalf("last event = %+v, want canceled finish", events[len(events)-1])
	}
	if err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: run.ID}); err != nil {
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
			Interaction: agent.Question{InterruptID: "question_1", Title: "Choose", Fields: []agent.QuestionField{{ID: "name", Label: "Name", Kind: agent.QuestionText, Required: true}}},
			Continue: func(agent.Answer) []Step {
				if _, err := runtime.ListSessions(t.Context(), agent.SessionQuery{}); err != nil {
					panic(err)
				}
				close(entered)
				<-release
				return completedScript().Prelude
			},
		}
	}
	run, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	events, err := followAll(t, runtime, run.ID, run.StartedAfter)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := events[len(events)-1].Event.(agent.RunInterrupted)
	resumeErr := make(chan error, 1)
	go func() {
		resumeErr <- runtime.ResumeRun(t.Context(), agent.ResumeRun{
			RunID: run.ID, InterruptID: agent.InteractionID(interrupted.Interaction),
			Answer: agent.QuestionAnswer{Values: map[string][]string{"name": {"cache"}}},
		})
	}()
	<-entered
	if err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-resumeErr; !errors.Is(err, agent.ErrRunCanceled) {
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
	return Script{Prelude: []Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
}
