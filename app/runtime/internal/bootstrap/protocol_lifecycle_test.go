package bootstrap

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	runtimeserver "github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// TestProtocolLifecycleSurvivesColdRestart is the protocol conformance smoke
// test over the real composition root and a fresh SQLite store. It deliberately
// crosses every control boundary that is easy to make locally correct while
// breaking as a whole: start, structured steer, wait, resume, cancel, process
// shutdown, startup recovery, and cold reads.
func TestProtocolLifecycleSurvivesColdRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LYRA_HOME", home)

	model := newLifecycleModel()
	host, api := openProtocolRuntime(t, model)
	firstOpen := true
	defer func() {
		if firstOpen {
			api.Close()
			if err := host.Close(); err != nil {
				t.Errorf("close first runtime: %v", err)
			}
		}
	}()

	ctx := protocol.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "protocol lifecycle",
	})
	if err != nil {
		t.Fatalf("sessions.create: %v", err)
	}

	started, startEvents, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Start the lifecycle check.",
		}},
	})
	if err != nil {
		t.Fatalf("runs.start: %v", err)
	}
	if started.RunID == "" || started.SegmentID == "" || started.UserItemID == "" {
		t.Fatalf("runs.start returned incomplete identity: %+v", started)
	}
	startEventsDone := collectRunEvents(startEvents)
	waitForSignal(t, model.firstCallStarted, "first model call")

	const steeringText = "Keep the resumed answer concise."
	if err := api.SteerRun(ctx, protocol.SteerRunRequest{
		RunID:             started.RunID,
		ExpectedSegmentID: started.SegmentID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: steeringText,
		}},
	}); err != nil {
		t.Fatalf("runs.steer: %v", err)
	}
	close(model.releaseFirstCall)
	waitForRunEvents(t, startEventsDone, "waiting segment")

	waiting, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatalf("runs.get waiting run: %v", err)
	}
	if waiting.Status != protocol.RunStatusWaiting || waiting.Outcome != nil || waiting.ActiveSegmentID != "" {
		t.Fatalf("waiting run = %+v, want waiting without outcome or active segment", waiting)
	}
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{
		RootRunID: started.RunID,
	})
	if err != nil {
		t.Fatalf("interrupts.list: %v", err)
	}
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("pending interrupts = %+v, want one complete set with one interrupt", pending.Data)
	}
	question := pending.Data[0].Interrupts[0]
	if question.RunID != started.RunID || question.Type != protocol.InterruptQuestion {
		t.Fatalf("pending interrupt = %+v, want this run's question", question)
	}

	resumed, resumeEvents, err := api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: question.ItemID,
			Response: protocol.InterruptResponseValue{
				Type:    protocol.InterruptResponseAnswer,
				Answers: [][]string{{"Yes"}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("runs.resume: %v", err)
	}
	if resumed.RunID != started.RunID || resumed.SegmentID == "" || resumed.SegmentID == started.SegmentID {
		t.Fatalf("runs.resume identity = %+v, want same run and a fresh segment", resumed)
	}
	if resumed.UserItemID != nil {
		t.Fatalf("runs.resume userItemId = %q without resume input", *resumed.UserItemID)
	}
	resumeEventsDone := collectRunEvents(resumeEvents)
	waitForSignal(t, model.resumedCallStarted, "resumed model call")

	canceled, err := api.CancelRun(ctx, protocol.CancelRunRequest{
		RunID: started.RunID, Reason: "conformance smoke complete",
	})
	if err != nil {
		t.Fatalf("runs.cancel: %v", err)
	}
	if canceled.Type != protocol.CancelRunRoot ||
		canceled.Run.ID != started.RunID ||
		canceled.Run.Status != protocol.RunStatusFinished ||
		canceled.Run.Outcome == nil ||
		canceled.Run.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("runs.cancel result = %+v, want finished(canceled) root", canceled)
	}
	waitForRunEvents(t, resumeEventsDone, "canceled segment")

	api.Close()
	if err := host.Close(); err != nil {
		t.Fatalf("close first runtime: %v", err)
	}
	firstOpen = false

	restartedHost, restartedAPI := openProtocolRuntime(t, newReplyStub("unused"))
	defer func() {
		restartedAPI.Close()
		if err := restartedHost.Close(); err != nil {
			t.Errorf("close restarted runtime: %v", err)
		}
	}()

	recovered, err := restartedAPI.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatalf("runs.get after restart: %v", err)
	}
	if recovered.Status != protocol.RunStatusFinished ||
		recovered.Outcome == nil ||
		recovered.Outcome.Type != protocol.OutcomeCanceled ||
		recovered.ActiveSegmentID != "" {
		t.Fatalf("cold run = %+v, want finished(canceled) without active segment", recovered)
	}
	items, err := restartedAPI.ListItems(ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{
			Type:      protocol.ItemScopeSession,
			SessionID: session.ID,
		},
	})
	if err != nil {
		t.Fatalf("items.list after restart: %v", err)
	}
	if !hasUserText(items.Data, "Start the lifecycle check.") {
		t.Fatal("cold items do not contain the opening user message")
	}
	if !hasUserText(items.Data, steeringText) {
		t.Fatal("cold items do not contain the structured steering message")
	}
}

type lifecycleModel struct {
	calls              atomic.Int32
	firstCallStarted   chan struct{}
	releaseFirstCall   chan struct{}
	resumedCallStarted chan struct{}
	firstStartedOnce   sync.Once
	resumedStartedOnce sync.Once
}

func newLifecycleModel() *lifecycleModel {
	return &lifecycleModel{
		firstCallStarted:   make(chan struct{}),
		releaseFirstCall:   make(chan struct{}),
		resumedCallStarted: make(chan struct{}),
	}
}

func (m *lifecycleModel) Call(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
	switch m.calls.Add(1) {
	case 1:
		m.firstStartedOnce.Do(func() { close(m.firstCallStarted) })
		select {
		case <-m.releaseFirstCall:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID:        "call_question",
			Name:      "ask_user",
			Arguments: `{"questions":[{"question":"Continue the lifecycle check?"}]}`,
		}))
		return chat.NewResponse(chat.Choice{
			Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls,
		})
	default:
		m.resumedStartedOnce.Do(func() { close(m.resumedCallStarted) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func (m *lifecycleModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		response, err := m.Call(ctx, request)
		yield(response, err)
	}
}

type noBoundaryMaintenance struct{}

func (noBoundaryMaintenance) Maintain(
	context.Context,
	turn.BoundaryMaintenanceInput,
) turn.BoundaryMaintenanceResult {
	return turn.BoundaryMaintenanceResult{}
}

func openProtocolRuntime(t *testing.T, model chat.Model) (*Host, *runtimeserver.Server) {
	t.Helper()
	stores, err := persistence.Open()
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		_ = stores.Close()
		t.Fatalf("build chat client: %v", err)
	}
	cfg := ComposeConfig(
		config.Settings{},
		stores,
		client,
		stores.Provider,
		NewHookResolver(stores.Trust),
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
	)
	cfg.DefaultCwd = stores.Home
	cfg.Maintenance = noBoundaryMaintenance{}

	assembly := NewAssembly(cfg)
	host, err := BuildAssembly(t.Context(), assembly)
	if err != nil {
		_ = CloseAssembly(assembly)
		t.Fatalf("build runtime: %v", err)
	}
	if err := RecoverStartup(t.Context(), host.Stack); err != nil {
		_ = host.Close()
		t.Fatalf("recover runtime: %v", err)
	}
	api, err := protocolServer(host.Stack, stores.Home)
	if err != nil {
		_ = host.Close()
		t.Fatalf("build protocol server: %v", err)
	}
	return host, api
}

func protocolServer(stack Stack, cwd string) (*runtimeserver.Server, error) {
	return runtimeserver.New(runtimeserver.Config{
		Sessions:           stack.Sessions,
		Integrations:       stack.Integrations,
		Approvals:          stack.Approvals,
		Models:             stack.Models,
		Tools:              stack.Tools,
		Codebase:           stack.Codebase,
		Coordinator:        stack.Coordinator,
		FileChanges:        stack.FileChanges,
		MCPStatus:          stack.MCPStatus,
		SkillChanges:       stack.SkillChanges,
		ScheduleFires:      stack.ScheduleFires,
		Changes:            stack.Changes,
		Queries:            stack.Queries,
		Usage:              stack.Usage,
		Feedback:           stack.Feedback,
		Schedules:          stack.Schedules,
		ScheduleFiring:     stack.ScheduleFiring,
		Goals:              stack.Goals,
		AgentMemory:        stack.AgentMemory,
		WorkspaceFiles:     stack.WorkspaceFiles,
		WorkspaceVCS:       stack.WorkspaceVCS,
		WorkspaceDiscovery: stack.WorkspaceDiscovery,
		WorkspaceKnowledge: stack.WorkspaceKnowledge,
		WorkspaceSkills:    stack.WorkspaceSkills,
		WorkspaceHooks:     stack.WorkspaceHooks,
		WorkspaceWatch:     stack.WorkspaceWatch,
		GitAvailable:       stack.GitAvailable,
		PlanEnabled:        stack.PlanEnabled,
		ServerInfo: protocol.ServerInfo{
			Name: "conformance-test", Version: "0.0.0-test",
			DefaultWorkspace: protocol.WorkspaceRef{Path: cwd}, Home: cwd,
		},
	})
}

func collectRunEvents(events iter.Seq[protocol.RunEvent]) <-chan []protocol.RunEvent {
	done := make(chan []protocol.RunEvent, 1)
	go func() {
		var collected []protocol.RunEvent
		for event := range events {
			collected = append(collected, event)
		}
		done <- collected
	}()
	return done
}

func waitForRunEvents(t *testing.T, done <-chan []protocol.RunEvent, phase string) []protocol.RunEvent {
	t.Helper()
	select {
	case events := <-done:
		if len(events) == 0 {
			t.Fatalf("%s emitted no run events", phase)
		}
		return events
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s events", phase)
		return nil
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, phase string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", phase)
	}
}

func hasUserText(items []protocol.Item, want string) bool {
	for _, item := range items {
		if item.Type != protocol.ItemTypeUserMessage {
			continue
		}
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText && block.Text == want {
				return true
			}
		}
	}
	return false
}
