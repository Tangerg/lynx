package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestRuntimeStartResumeAndColdRestore(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	opened, conversation := startWaitingRun(t, runtime, session.ID)
	interaction := requireWaitingProjection(t, runtime, session.ID, opened)
	resumeApprovedRun(t, runtime, opened, conversation, interaction)
	requireCompletedColdProjection(t, runtime, session.ID, opened.RunID, interaction)
}

func TestApprovalCatalogRejectsEmptyIdentitiesConsistently(t *testing.T) {
	t.Parallel()

	runtime := New()
	if _, err := runtime.ListApprovalRules(t.Context(), "  "); err == nil {
		t.Fatal("empty session identity was accepted")
	}
	if err := runtime.DeleteApprovalRule(t.Context(), "\t"); err == nil {
		t.Fatal("empty rule identity was accepted")
	}
}

func TestSessionCatalogRejectsInvalidLocalFilters(t *testing.T) {
	t.Parallel()

	runtime := New()
	for _, query := range []agent.SessionQuery{
		{Limit: -1},
		{Workspace: "relative/workspace"},
	} {
		if _, err := runtime.ListSessions(t.Context(), query); err == nil {
			t.Fatalf("ListSessions accepted %+v", query)
		}
	}
}

func TestProjectApprovalRulesFollowTheResolvedProjectRoot(t *testing.T) {
	t.Parallel()

	runtime := New()
	runtime.mu.Lock()
	runtime.sessions["ses_demo_1"].meta.Workspace.ProjectRoot = "/tmp/demo"
	runtime.sessions["ses_demo_2"].meta.Workspace.ProjectRoot = "/tmp/demo"
	runtime.rules = []storedRule{{view: agent.ApprovalRule{
		ID: "rule_project", Scope: agent.RememberProject, Dir: "/tmp/demo",
		Tool: "shell", Subject: "go test ./...", Decision: agent.ApprovalRuleAllow,
	}}}
	runtime.mu.Unlock()

	for _, sessionID := range []string{"ses_demo_1", "ses_demo_2"} {
		rules, err := runtime.ListApprovalRules(t.Context(), sessionID)
		if err != nil || len(rules) != 1 {
			t.Fatalf("project rules for %s = %+v, %v", sessionID, rules, err)
		}
	}
	rules, err := runtime.ListApprovalRules(t.Context(), "ses_demo_3")
	if err != nil || len(rules) != 0 {
		t.Fatalf("unrelated project rules = %+v, %v", rules, err)
	}
}

func startWaitingRun(t *testing.T, runtime *Runtime, sessionID string) (agent.SegmentStream, *agent.Conversation) {
	t.Helper()
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: sessionID, Message: agent.Message{Text: "fix the flaky test"},
		Options: agent.RunOptions{Provider: "mock", Model: "balanced", Limits: agent.RunLimits{MaxSteps: 12, MaxBudgetUSD: 1.5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	if conversation.Phase() != agent.ConversationWaiting || len(conversation.Interactions()) != 1 {
		t.Fatalf("after first segment: phase %v, interactions %d", conversation.Phase(), len(conversation.Interactions()))
	}
	return opened, conversation
}

func requireWaitingProjection(t *testing.T, runtime *Runtime, sessionID string, opened agent.SegmentStream) agent.Interaction {
	t.Helper()
	waiting, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(waiting.Interactions) != 1 {
		t.Fatalf("waiting interactions = %d, want 1", len(waiting.Interactions))
	}
	waitingRun, ok := waiting.LatestRun()
	if !ok {
		t.Fatal("waiting snapshot has no run")
	}
	interaction := waiting.Interactions[0]
	approvalItem, ok := snapshotBlock(waiting, opened.RunID, agent.InteractionItemID(interaction))
	if !ok || approvalItem.Kind != agent.BlockTool || approvalItem.Status != agent.BlockStatusRunning || waitingRun.Usage.InputTokens == 0 {
		t.Fatalf("waiting approval projection = item %+v, usage %+v", approvalItem, waitingRun.Usage)
	}
	return interaction
}

func resumeApprovedRun(t *testing.T, runtime *Runtime, opened agent.SegmentStream, conversation *agent.Conversation, interaction agent.Interaction) {
	t.Helper()
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: opened.RunID,
		Answers: []agent.InterruptAnswer{{
			ItemID: agent.InteractionItemID(interaction),
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, Remember: agent.RememberProject},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if continued.SegmentID == opened.SegmentID {
		t.Fatal("resume reused the first segment")
	}
	drain(t, continued, conversation)
	if conversation.Outcome().Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v", conversation.Outcome())
	}
}

func requireCompletedColdProjection(t *testing.T, runtime *Runtime, sessionID, runID string, interaction agent.Interaction) {
	t.Helper()
	snapshot, err := runtime.GetSession(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, active := snapshot.ActiveRun(); active || len(snapshot.Interactions) != 0 || len(snapshot.Transcript) < 4 {
		t.Fatalf("snapshot = runs %+v, interactions %d, transcript %d", snapshot.Runs, len(snapshot.Interactions), len(snapshot.Transcript))
	}
	if latest, ok := snapshot.LatestRun(); !ok || latest.Limits.MaxSteps != 12 || latest.Limits.MaxBudgetUSD != 1.5 {
		t.Fatalf("latest run limits = %+v", latest.Limits)
	}
	approvalItem, ok := snapshotBlock(snapshot, runID, agent.InteractionItemID(interaction))
	if !ok || approvalItem.Status != agent.BlockStatusCompleted || approvalItem.Tool.Status != agent.ToolOK {
		t.Fatalf("completed approval item = %+v", approvalItem)
	}
	rules, err := runtime.ListApprovalRules(t.Context(), sessionID)
	if err != nil || len(rules) != 1 || rules[0].Scope != agent.RememberProject {
		t.Fatalf("rules = %+v, %v", rules, err)
	}
}

func TestRuntimeReconnectUsesOpaqueReplayCheckpoint(t *testing.T) {
	runtime := New()
	runtime.Faults = []SubscriptionFault{{Kind: FaultDisconnect, After: 1}}
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			{Delay: 30 * time.Millisecond, Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "done"}}},
			{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	var disconnected error
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			disconnected = streamErr
			break
		}
		if _, err := conversation.ApplyRunEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(disconnected, agent.ErrDisconnected) {
		t.Fatalf("stream error = %v", disconnected)
	}
	checkpoint := conversation.Checkpoint()
	if checkpoint == "" {
		t.Fatal("no replay checkpoint was retained")
	}
	rebound, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{
		RunID: opened.RunID, SegmentID: opened.SegmentID, AfterEventID: checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, rebound, conversation)
	if conversation.Outcome().Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v", conversation.Outcome())
	}
}

func TestRuntimeSubscribeWithoutCheckpointAttachesAtHead(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{Delay: time.Second, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := timeLimitedContext(t, 20*time.Millisecond)
	defer cancel()
	attached, err := runtime.SubscribeRun(ctx, agent.SubscribeRun{RunID: opened.RunID, SegmentID: opened.SegmentID})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for event, streamErr := range attached.Events {
		if streamErr != nil {
			break
		}
		count++
		_ = event
	}
	if count != 0 {
		t.Fatalf("attach-at-head replayed %d historical events", count)
	}
	_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID})
}

func TestRuntimeForkStartsWithAFreshProjectionAtRunBoundary(t *testing.T) {
	runtime := New()
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_demo_1", FromRunID: "run_demo_history"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork projection = %d blocks, %d runs", len(snapshot.Transcript), len(snapshot.Runs))
	}
}

func TestRuntimeRollbackRestoresTheEarliestDroppedOpeningInput(t *testing.T) {
	runtime := New()
	result, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dropped) != 1 {
		t.Fatalf("dropped runs = %+v", result.Dropped)
	}
	input, ok := result.FirstOpeningInput()
	text, images := input.OpeningText()
	if !ok || text != "Why is the cache expiry test flaky?" || images != 0 {
		t.Fatalf("opening input = (%q, %d, %t)", text, images, ok)
	}
	snapshot, err := runtime.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runs) != 0 || len(snapshot.Transcript) != 0 {
		t.Fatalf("snapshot after rollback = %+v", snapshot)
	}
}

func TestRuntimeForkExcludesAnActiveTail(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: "ses_demo_1", Message: agent.Message{Text: "active tail"}})
	if err != nil {
		t.Fatal(err)
	}
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_demo_1"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork copied a parent projection: blocks=%+v runs=%+v", snapshot.Transcript, snapshot.Runs)
	}
	if _, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: "ses_demo_1", FromRunID: opened.RunID}); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("explicit active boundary error = %v", err)
	}
	_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID})
}

func TestRuntimeForkCopiesThePlanAtItsRunBoundary(t *testing.T) {
	runtime := New()
	runtime.Script = func(prompt string) Script {
		plan := []agent.PlanItem{{Title: prompt + " plan", Status: agent.PlanActive}}
		delay := time.Duration(0)
		if prompt == "active" {
			delay = time.Hour
		}
		return Script{Prelude: []Step{
			{Event: agent.PlanChanged{Items: plan}},
			{Delay: delay, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "boundary"}})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, first, agent.NewConversation())
	second, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "active"}})
	if err != nil {
		t.Fatal(err)
	}
	for event, streamErr := range second.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, changed := event.Event.(agent.PlanChanged); changed {
			break
		}
	}
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), forked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PlanRevision != 1 || len(snapshot.Plan) != 1 || snapshot.Plan[0].Title != "boundary plan" {
		t.Fatalf("fork plan = revision %d, items %+v", snapshot.PlanRevision, snapshot.Plan)
	}
	if len(snapshot.Transcript) != 0 || len(snapshot.Runs) != 0 {
		t.Fatalf("fork copied a parent projection: blocks=%+v runs=%+v", snapshot.Transcript, snapshot.Runs)
	}
	_, _ = runtime.CancelRun(t.Context(), agent.CancelRun{RunID: second.RunID})
}

func TestRuntimeColdReadTracksAndSettlesRunningItems(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{
			{Event: agent.BlockStarted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant}}},
			{Event: agent.BlockStarted{Block: agent.Block{ID: "tool", Kind: agent.BlockTool, Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning}}}},
			{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
		}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "run"}})
	if err != nil {
		t.Fatal(err)
	}
	startedCount := 0
	for event, streamErr := range opened.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, started := event.Event.(agent.BlockStarted); started {
			startedCount++
			if startedCount == 2 {
				break
			}
		}
	}
	running, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(running.Transcript) != 2 {
		t.Fatalf("cold transcript includes a provisional assistant start: %+v", running.Transcript)
	}
	if got := running.Transcript[len(running.Transcript)-1]; got.Status != agent.BlockStatusRunning || got.ID != opened.RunID+":tool" || got.Kind != agent.BlockTool {
		t.Fatalf("running item = %+v", got)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID}); err != nil {
		t.Fatal(err)
	}
	settled, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := settled.Transcript[len(settled.Transcript)-1]; got.Status != agent.BlockStatusIncomplete {
		t.Fatalf("settled item = %+v", got)
	}
}

func TestScriptContinuationReceivesFixtureLocalItemIDs(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	var received string
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{approvalFixture("approval", "approve")},
			Continue: func(answers []agent.InterruptAnswer) []Step {
				received = answers[0].ItemID
				return []Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	interaction := conversation.Interactions()[0]
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
		ItemID: agent.InteractionItemID(interaction), Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, continued, conversation)
	if received != "approval" {
		t.Fatalf("continuation item id = %q, want fixture-local id", received)
	}
}

func TestApprovalArgumentOverrideBecomesTheCompletedToolProjection(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	original := []byte(`{"command":"rm generated.txt"}`)
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval", Title: "Run command",
				Tool: &agent.ToolCall{
					Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning, ArgumentsJSON: original,
				},
			}},
			Continue: func([]agent.InterruptAnswer) []Step {
				return []Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: session.ID, Message: agent.Message{Text: "run safely"},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	interaction := conversation.Interactions()[0]
	override, err := agent.ParseToolArgumentOverride([]byte(`{"command":"echo safe"}`))
	if err != nil {
		t.Fatal(err)
	}
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{
		RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
			ItemID: agent.InteractionItemID(interaction),
			Answer: agent.ApprovalAnswer{Decision: agent.ApprovalApprove, ArgumentOverride: override},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, continued, conversation)
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	completed, ok := snapshotBlock(snapshot, opened.RunID, agent.InteractionItemID(interaction))
	if !ok || completed.Tool == nil || string(completed.Tool.ArgumentsJSON) != `{"command":"echo safe"}` {
		t.Fatalf("completed edited tool = %+v", completed)
	}
	if string(original) != `{"command":"rm generated.txt"}` {
		t.Fatalf("mock mutated fixture arguments: %s", original)
	}
}

func TestInvalidFaultConfigurationDoesNotMutateRunState(t *testing.T) {
	runtime := New()
	runtime.Faults = []SubscriptionFault{{Kind: FaultKind("unknown"), After: 1}}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	if _, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "start"}}); err == nil {
		t.Fatal("invalid subscription fault was ignored")
	}
	snapshot, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Runs) != 0 || len(snapshot.Transcript) != 0 || snapshot.Session.Status != agent.SessionIdle {
		t.Fatalf("failed start mutated snapshot: %+v", snapshot)
	}
}

func TestRememberedRulesRemoveOnlyMatchedApprovalsFromThePendingSet(t *testing.T) {
	runtime := New()
	runtime.Instant = true
	runtime.rules = []storedRule{{view: agent.ApprovalRule{
		ID: "rule_1", Scope: agent.RememberGlobal, Tool: "shell", Subject: "go test ./...", Decision: agent.ApprovalRuleAllow,
	}}}
	var continuedWith []agent.InterruptAnswer
	runtime.Script = func(string) Script {
		return Script{
			Interactions: []agent.Interaction{
				func() agent.Approval {
					approval := approvalFixture("approval", "run tests")
					approval.RuleHint, approval.Rememberable = "shell:go test ./...", true
					return approval
				}(),
				agent.Question{ItemID: "question", Title: "Target", Fields: []agent.QuestionField{{Prompt: "Target", Kind: agent.QuestionText}}},
			},
			Continue: func(answers []agent.InterruptAnswer) []Step {
				continuedWith = cloneAnswers(answers)
				return []Step{{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}
			},
		}
	}
	session, _ := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: "/tmp/mock"})
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	conversation := agent.NewConversation()
	drain(t, opened, conversation)
	pending := conversation.Interactions()
	if len(pending) != 1 {
		t.Fatalf("pending interactions = %+v, want only the unmatched question", pending)
	}
	question, ok := pending[0].(agent.Question)
	if !ok {
		t.Fatalf("pending interaction = %T, want question", pending[0])
	}
	waiting, err := runtime.GetSession(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	questionItem, found := snapshotBlock(waiting, opened.RunID, question.ItemID)
	if !found || questionItem.Kind != agent.BlockQuestion || questionItem.Question == nil {
		t.Fatalf("durable question item = %+v", questionItem)
	}
	continued, err := runtime.ResumeRun(t.Context(), agent.ResumeRun{RunID: opened.RunID, Answers: []agent.InterruptAnswer{{
		ItemID: question.ItemID, Answer: agent.QuestionAnswer{Values: [][]string{{"linux"}}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, continued, conversation)
	if len(continuedWith) != 2 || continuedWith[0].ItemID != "approval" || continuedWith[1].ItemID != "question" {
		t.Fatalf("continuation answers = %+v, want the complete fixture-local set", continuedWith)
	}
}

func drain(t *testing.T, stream agent.SegmentStream, conversation *agent.Conversation) {
	t.Helper()
	if err := stream.Validate(); err != nil {
		t.Fatal(err)
	}
	for event, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		if _, err := conversation.ApplyRunEvent(event); err != nil {
			t.Fatal(err)
		}
	}
}

func approvalFixture(itemID, title string) agent.Approval {
	return agent.Approval{
		ItemID: itemID, Title: title,
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
}

func snapshotBlock(snapshot agent.SessionSnapshot, runID, itemID string) (agent.Block, bool) {
	for _, block := range snapshot.Transcript {
		if block.RunID == runID && block.ID == itemID {
			return block, true
		}
	}
	return agent.Block{}, false
}

func timeLimitedContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), timeout)
}
