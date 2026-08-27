package bootstrap

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/scope/app/runtime/internal/config"
	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

const modelCallsBeforeMidRunCompaction = 12

func TestRuntimeCompactsDuringOneLongRunBeforeTheNextMainModelCall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SCOPEAPP_HOME", home)
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: home, DefaultWorkspacePath: home,
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &longContextModel{}
	client, err := chatclient.New(model, chatclient.Config{Streamer: model})
	if err != nil {
		t.Fatal(err)
	}
	cfg := ComposeConfig(
		config.Settings{Provider: "anthropic", Model: "claude-test"},
		stores,
		client,
		stores.Providers,
		NewHookResolver(stores.DataDirectory, stores.Trust),
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
	)
	cfg.UserHome = home
	cfg.DefaultWorkspacePath = home
	cfg.Maintenance = noMaintenance{}
	host, api := buildProtocolRuntime(t, cfg, home)
	defer func() {
		api.Close()
		if closeErr := host.Close(); closeErr != nil {
			t.Errorf("close runtime: %v", closeErr)
		}
	}()
	ctx := operation.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
	})
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "mid-run compaction",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, sequence, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Exercise one long Tool loop without ending the Run.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := waitForRunEvents(t, collectRunEvents(sequence), "long compacting Run")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != protocol.RunStatusFinished || finished.Outcome == nil ||
		finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run = %+v, want completed", finished)
	}
	if !eventsContainCompaction(events) {
		t.Fatalf("Run events contain no mid-run compaction boundary: %+v", events)
	}
	second, secondSequence, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Continue from the compacted history in a fresh Run.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(secondSequence), "post-compaction fresh Run")
	secondFinished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: second.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if secondFinished.Status != protocol.RunStatusFinished || secondFinished.Outcome == nil ||
		secondFinished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("fresh Run after compaction = %+v, want completed", secondFinished)
	}
	mainCalls, summaryCalls, compactedMainCalls := model.Snapshot()
	if mainCalls != modelCallsBeforeMidRunCompaction+2 || summaryCalls != 1 || compactedMainCalls != 2 {
		t.Fatalf(
			"model calls = main:%d summary:%d with_compacted_context:%d",
			mainCalls,
			summaryCalls,
			compactedMainCalls,
		)
	}
	history, err := stores.ChatHistory.Read(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) >= 1+2*modelCallsBeforeMidRunCompaction ||
		!strings.HasPrefix(history[0].Text(), "[Earlier conversation summary]") ||
		history[len(history)-1].Text() != "long Run completed after compaction" {
		t.Fatalf("durable history was not compacted in place: %#v", history)
	}
}

type longContextModel struct {
	mu                 sync.Mutex
	mainCalls          int
	summaryCalls       int
	compactedMainCalls int
}

func (l *longContextModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if isCompactionRequest(request) {
		l.summaryCalls++
		return completedTextResponse("## Goal\nKeep the long Run stable.\n\n## Progress\nTool rounds completed."), nil
	}
	l.mainCalls++
	if l.mainCalls <= modelCallsBeforeMidRunCompaction {
		if !hasToolDefinition(request.Tools, tool.GetGoal) {
			return nil, fmt.Errorf("main model request does not expose %s", tool.GetGoal)
		}
		call := chat.ToolCall{
			ID: fmt.Sprintf("call_goal_%02d", l.mainCalls), Name: tool.GetGoal, Arguments: `{}`,
		}
		message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
		return chat.NewResponse(&chat.Output{
			Message: &message, FinishReason: chat.FinishReasonToolCalls,
		}, nil)
	}
	sawSummary := false
	for index := range request.Messages {
		if strings.HasPrefix(request.Messages[index].Text(), "[Earlier conversation summary]") {
			sawSummary = true
			break
		}
	}
	if !sawSummary {
		return nil, fmt.Errorf("main call %d reached the provider without compacted context", l.mainCalls)
	}
	l.compactedMainCalls++
	return completedTextResponse("long Run completed after compaction"), nil
}

func (l *longContextModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		response, err := l.Call(ctx, request)
		yield(response, err)
	}
}

func (l *longContextModel) Snapshot() (mainCalls int, summaryCalls int, compactedMainCalls int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.mainCalls, l.summaryCalls, l.compactedMainCalls
}

func isCompactionRequest(request *chat.Request) bool {
	return request != nil && len(request.Messages) > 0 && len(request.Tools) == 0
}

func hasToolDefinition(definitions []chat.ToolDefinition, name string) bool {
	for _, definition := range definitions {
		if definition.Name == name {
			return true
		}
	}
	return false
}

func completedTextResponse(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Output: &chat.Output{
		Message: &message, FinishReason: chat.FinishReasonStop,
	}}
}

func eventsContainCompaction(events []protocol.RunEvent) bool {
	for _, event := range events {
		if event.Event.Item != nil && event.Event.Item.Type == protocol.ItemTypeCompaction {
			return true
		}
	}
	return false
}
