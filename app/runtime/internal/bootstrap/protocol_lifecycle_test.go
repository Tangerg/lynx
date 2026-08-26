package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/config"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	runtimeserver "github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/protocol"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
)

// TestProtocolLifecycleSurvivesColdRestart is the protocol conformance smoke
// test over the real composition root and a fresh SQLite store. It deliberately
// crosses every control boundary that is easy to make locally correct while
// breaking as a whole: start, structured steer, wait, resume, cancel, runtime
// shutdown, startup recovery, and cold reads.
func TestProtocolLifecycleSurvivesColdRestart(t *testing.T) {
	fixture := newProtocolLifecycleFixture(t)
	defer fixture.closeFirstRuntime()
	question := fixture.startAndPark()
	fixture.resumeAndCancel(question)
	fixture.assertColdState()
}

func TestAssemblyPreservesParkedQuestionAcrossCrashLikeRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("LYRA_HOME", home)
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: home, DefaultWorkspacePath: home,
	})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	model := newLifecycleModel()
	cfg := protocolRuntimeConfig(t, stores, model)
	firstHost, firstAPI := buildProtocolRuntime(t, cfg, stores.DataDirectory)
	fixture := &protocolLifecycleFixture{
		t: t, home: home, ctx: protocolLifecycleContext(t.Context()), model: model,
		host: firstHost, api: firstAPI, stores: stores,
	}
	defer fixture.closeFirstRuntime()
	fixture.startAndPark()

	restartedConfig := cfg
	restartedConfig.Resources = nil
	restartedHost, restartedAPI := buildProtocolRuntime(t, restartedConfig, stores.DataDirectory)
	defer func() {
		restartedAPI.Close()
		if closeErr := restartedHost.Close(); closeErr != nil {
			t.Errorf("close restarted runtime: %v", closeErr)
		}
	}()
	restarted, err := restartedAPI.GetRun(fixture.ctx, protocol.GetRunRequest{RunID: fixture.started.RunID})
	if err != nil {
		t.Fatalf("runs.get after crash-like restart: %v", err)
	}
	pending, err := restartedAPI.ListInterrupts(fixture.ctx, protocol.ListInterruptsRequest{
		RootRunID: fixture.started.RunID,
	})
	if err != nil {
		t.Fatalf("interrupt.list after crash-like restart: %v", err)
	}
	if restarted.Status != protocol.RunStatusWaiting || restarted.Outcome != nil ||
		len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("restarted waiting boundary = run %+v, interrupts %+v", restarted, pending.Data)
	}
	fixture.resumeAndCancelWith(restartedAPI, pending.Data[0].Interrupts[0], false)
}

type protocolLifecycleFixture struct {
	t         *testing.T
	home      string
	ctx       context.Context
	model     *lifecycleModel
	host      *Host
	api       *runtimeserver.Server
	stores    *persistence.Bundle
	sessionID string
	started   *protocol.StartRunResponse
	closed    bool
}

func newProtocolLifecycleFixture(t *testing.T) *protocolLifecycleFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("LYRA_HOME", home)
	model := newLifecycleModel()
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: home, DefaultWorkspacePath: home,
	})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	cfg := protocolRuntimeConfig(t, stores, model)
	host, api := buildProtocolRuntime(t, cfg, stores.DataDirectory)
	ctx := protocolLifecycleContext(t.Context())
	return &protocolLifecycleFixture{
		t: t, home: home, ctx: ctx, model: model, host: host, api: api, stores: stores,
	}
}

func protocolLifecycleContext(ctx context.Context) context.Context {
	return operation.WithRequestMeta(ctx, protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientCapabilities: &protocol.ClientCapabilities{
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	})
}

func (p *protocolLifecycleFixture) startAndPark() protocol.Interrupt {
	p.t.Helper()
	session, err := p.api.CreateSession(p.ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: p.home}, Title: "protocol lifecycle",
	})
	if err != nil {
		p.t.Fatalf("sessions.create: %v", err)
	}
	p.sessionID = session.ID
	started, startEvents, err := p.api.StartRun(p.ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Start the lifecycle check.",
		}},
	})
	if err != nil {
		p.t.Fatalf("runs.start: %v", err)
	}
	p.started = started
	if started.RunID == "" || started.SegmentID == "" || started.UserItemID == "" {
		p.t.Fatalf("runs.start returned incomplete identity: %+v", started)
	}
	startEventsDone := collectRunEvents(startEvents)
	select {
	case <-p.model.firstCallStarted:
	case events := <-startEventsDone:
		ended, _ := p.api.GetRun(p.ctx, protocol.GetRunRequest{RunID: started.RunID})
		diagnostic, _ := json.Marshal(ended)
		p.t.Fatalf(
			"run ended before first model call: run=%s domainFailure=%s events=%+v",
			diagnostic,
			p.domainFailureDiagnostic(started.RunID),
			events,
		)
	case <-time.After(5 * time.Second):
		p.t.Fatal("timed out waiting for first model call")
	}

	if steerRunErr := p.api.SteerRun(p.ctx, protocol.SteerRunRequest{
		RunID:             started.RunID,
		ExpectedSegmentID: started.SegmentID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: protocolLifecycleSteeringText,
		}},
	}); steerRunErr != nil {
		p.t.Fatalf("runs.steer: %v", steerRunErr)
	}
	close(p.model.releaseFirstCall)
	waitForRunEvents(p.t, startEventsDone, "waiting segment")

	waiting, err := p.api.GetRun(p.ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		p.t.Fatalf("runs.get waiting run: %v", err)
	}
	if waiting.Status != protocol.RunStatusWaiting || waiting.Outcome != nil || waiting.ActiveSegmentID != "" {
		p.t.Fatalf("waiting run = %+v, want waiting without outcome or active segment", waiting)
	}
	pending, err := p.api.ListInterrupts(p.ctx, protocol.ListInterruptsRequest{
		RootRunID: started.RunID,
	})
	if err != nil {
		p.t.Fatalf("interrupt.list: %v", err)
	}
	if len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		p.t.Fatalf("pending interrupts = %+v, want one complete set with one interrupt", pending.Data)
	}
	question := pending.Data[0].Interrupts[0]
	if question.RunID != started.RunID || question.Type != protocol.InterruptQuestion {
		p.t.Fatalf("pending interrupt = %+v, want this run's question", question)
	}
	return question
}

func (p *protocolLifecycleFixture) domainFailureDiagnostic(runID string) string {
	if p.stores == nil || p.stores.Runs == nil {
		return "unavailable"
	}
	value, found, err := p.stores.Runs.Run(p.ctx, runID)
	if err != nil {
		return "read error: " + err.Error()
	}
	if !found {
		return "missing"
	}
	return fmt.Sprintf("%+v", value.Snapshot().Failure)
}

func (p *protocolLifecycleFixture) resumeAndCancel(question protocol.Interrupt) {
	p.resumeAndCancelWith(p.api, question, true)
}

func (p *protocolLifecycleFixture) resumeAndCancelWith(
	api *runtimeserver.Server,
	question protocol.Interrupt,
	closeFirst bool,
) {
	p.t.Helper()
	resumed, resumeEvents, err := api.ResumeRun(p.ctx, protocol.ResumeRunRequest{
		RunID: p.started.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: question.ItemID,
			Response: protocol.InterruptResponseValue{
				Type:    protocol.InterruptResponseAnswer,
				Answers: [][]string{{"Yes"}},
			},
		}},
	})
	if err != nil {
		p.t.Fatalf("runs.resume: %v", err)
	}
	if resumed.RunID != p.started.RunID || resumed.SegmentID == "" || resumed.SegmentID == p.started.SegmentID {
		p.t.Fatalf("runs.resume identity = %+v, want same run and a fresh segment", resumed)
	}
	if resumed.UserItemID != nil {
		p.t.Fatalf("runs.resume userItemId = %q without resume input", *resumed.UserItemID)
	}
	resumeEventsDone := collectRunEvents(resumeEvents)
	waitForSignal(p.t, p.model.resumedCallStarted, "resumed model call")
	items, err := api.ListItems(p.ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: p.started.RunID},
	})
	if err != nil {
		p.t.Fatalf("items.list after accepted answer: %v", err)
	}
	var accepted bool
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeQuestion && item.Question != nil &&
			len(item.Question.Answers) == 1 && len(item.Question.Answers[0]) == 1 &&
			item.Question.Answers[0][0] == "Yes" {
			accepted = true
		}
	}
	if !accepted {
		p.t.Fatalf("items after accepted answer = %+v, want durable Question answers", items.Data)
	}

	type cancelResult struct {
		response *protocol.CancelRunResponse
		err      error
	}
	cancelDone := make(chan cancelResult, 1)
	go func() {
		response, cancelErr := api.CancelRun(p.ctx, protocol.CancelRunRequest{
			RunID: p.started.RunID, Reason: "conformance smoke complete",
		})
		cancelDone <- cancelResult{response: response, err: cancelErr}
	}()
	settlementDelay := time.NewTimer(50 * time.Millisecond)
	<-settlementDelay.C
	close(p.model.releaseResumedCall)
	cancelCall := <-cancelDone
	canceled, err := cancelCall.response, cancelCall.err
	if err != nil {
		p.t.Fatalf("runs.cancel: %v", err)
	}
	if canceled.Type != protocol.CancelRunRoot ||
		canceled.Run.ID != p.started.RunID ||
		canceled.Run.Status != protocol.RunStatusFinished ||
		canceled.Run.Outcome == nil ||
		canceled.Run.Outcome.Type != protocol.OutcomeCanceled {
		p.t.Fatalf("runs.cancel result = %+v, want finished(canceled) root", canceled)
	}
	waitForRunEvents(p.t, resumeEventsDone, "canceled segment")
	if closeFirst {
		p.closeFirstRuntime()
	}
}

func (p *protocolLifecycleFixture) assertColdState() {
	p.t.Helper()
	restartedHost, restartedAPI := openProtocolRuntime(p.t, newReplyStub("unused"))
	defer func() {
		restartedAPI.Close()
		if err := restartedHost.Close(); err != nil {
			p.t.Errorf("close restarted runtime: %v", err)
		}
	}()

	recovered, err := restartedAPI.GetRun(p.ctx, protocol.GetRunRequest{RunID: p.started.RunID})
	if err != nil {
		p.t.Fatalf("runs.get after restart: %v", err)
	}
	if recovered.Status != protocol.RunStatusFinished ||
		recovered.Outcome == nil ||
		recovered.Outcome.Type != protocol.OutcomeCanceled ||
		recovered.ActiveSegmentID != "" {
		p.t.Fatalf("cold run = %+v, want finished(canceled) without active segment", recovered)
	}
	items, err := restartedAPI.ListItems(p.ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{
			Type:      protocol.ItemScopeSession,
			SessionID: p.sessionID,
		},
	})
	if err != nil {
		p.t.Fatalf("items.list after restart: %v", err)
	}
	if !hasUserText(items.Data, "Start the lifecycle check.") {
		p.t.Fatal("cold items do not contain the opening user message")
	}
	if !hasUserText(items.Data, protocolLifecycleSteeringText) {
		p.t.Fatal("cold items do not contain the structured steering message")
	}
}

func (p *protocolLifecycleFixture) closeFirstRuntime() {
	p.t.Helper()
	if p.closed {
		return
	}
	p.api.Close()
	if err := p.host.Close(); err != nil {
		p.t.Fatalf("close first runtime: %v", err)
	}
	p.closed = true
}

const protocolLifecycleSteeringText = "Keep the resumed answer concise."

type lifecycleModel struct {
	calls              atomic.Int32
	firstCallStarted   chan struct{}
	releaseFirstCall   chan struct{}
	resumedCallStarted chan struct{}
	releaseResumedCall chan struct{}
	firstStartedOnce   sync.Once
	resumedStartedOnce sync.Once
}

func newLifecycleModel() *lifecycleModel {
	return &lifecycleModel{
		firstCallStarted:   make(chan struct{}),
		releaseFirstCall:   make(chan struct{}),
		resumedCallStarted: make(chan struct{}),
		releaseResumedCall: make(chan struct{}),
	}
}

func (l *lifecycleModel) Call(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
	switch l.calls.Add(1) {
	case 1:
		l.firstStartedOnce.Do(func() { close(l.firstCallStarted) })
		select {
		case <-l.releaseFirstCall:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
			ID:        "call_question",
			Name:      "ask_user",
			Arguments: `{"questions":[{"question":"Continue the lifecycle check?"}]}`,
		}))
		return chat.NewResponse(&chat.Output{
			Message: &message, FinishReason: chat.FinishReasonToolCalls,
		}, nil)
	default:
		l.resumedStartedOnce.Do(func() { close(l.resumedCallStarted) })
		select {
		case <-l.releaseResumedCall:
			message := chat.NewAssistantMessage(chat.NewTextPart("settled before cancellation"))
			return chat.NewResponse(&chat.Output{Message: &message}, nil)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (l *lifecycleModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		response, err := l.Call(ctx, request)
		yield(response, err)
	}
}

type noMaintenance struct{}

func (noMaintenance) Maintain(
	context.Context,
	agentexec.RunMaintenanceInput,
) agentexec.RunMaintenanceResult {
	return agentexec.RunMaintenanceResult{}
}

func openProtocolRuntime(t *testing.T, model chat.Model) (*Host, *runtimeserver.Server) {
	t.Helper()
	dataDirectory := os.Getenv("LYRA_HOME")
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory:        dataDirectory,
		DefaultWorkspacePath: dataDirectory,
	})
	if err != nil {
		t.Fatalf("open persistence: %v", err)
	}
	cfg := protocolRuntimeConfig(t, stores, model)
	return buildProtocolRuntime(t, cfg, stores.DataDirectory)
}

func protocolRuntimeConfig(t *testing.T, stores *persistence.Bundle, model chat.Model) Config {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		_ = stores.Close()
		t.Fatalf("build chat client: %v", err)
	}
	cfg := ComposeConfig(
		config.Settings{Provider: "anthropic", Model: "claude-test"},
		stores,
		client,
		stores.Providers,
		NewHookResolver(stores.DataDirectory, stores.Trust),
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
	)
	cfg.UserHome = stores.DataDirectory
	cfg.DefaultWorkspacePath = stores.DataDirectory
	cfg.Maintenance = noMaintenance{}
	return cfg
}

func buildProtocolRuntime(t *testing.T, cfg Config, cwd string) (*Host, *runtimeserver.Server) {
	t.Helper()
	assembly := NewAssembly(t.Context(), cfg)
	host, err := BuildAssembly(t.Context(), assembly)
	if err != nil {
		_ = CloseAssembly(assembly)
		t.Fatalf("build runtime: %v", err)
	}
	if recoverStartupErr := host.application.recoverStartup(t.Context()); recoverStartupErr != nil {
		_ = host.Close()
		t.Fatalf("recover runtime: %v", recoverStartupErr)
	}
	api, err := protocolServer(host, cwd)
	if err != nil {
		_ = host.Close()
		t.Fatalf("build protocol server: %v", err)
	}
	return host, api
}

func protocolServer(host *Host, cwd string) (*runtimeserver.Server, error) {
	return host.application.newOperationService(protocol.ServerInfo{
		Name: "conformance-test", Version: "0.0.0-test", InstanceID: "runtime_test",
		DefaultWorkspace: protocol.WorkspaceRef{Path: cwd}, Home: cwd,
	}, "idp_protocol_lifecycle_test")
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
