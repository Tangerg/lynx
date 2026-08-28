package bootstrap

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/persistence"
	planapp "github.com/Tangerg/scope/app/runtime/internal/application/plans"
	"github.com/Tangerg/scope/app/runtime/internal/config"
	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	runtimeserver "github.com/Tangerg/scope/app/runtime/internal/delivery/server"
	plandomain "github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestRuntimeCompactionKeepsOnlyTheCurrentSessionPlan(t *testing.T) {
	model := &planChangingLongContextModel{}
	stores, api, ctx, home := newSessionStateE2ERuntime(t, model)
	sessionID := createSessionWithInitialPlan(t, stores, api, ctx, home, "Plan replacement across compaction")
	started, sequence, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: sessionID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "Replace the Plan, then keep this Run alive across context compaction.",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(sequence), "Plan-changing compacting Run")
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != protocol.RunStatusFinished || finished.Outcome == nil ||
		finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run = %+v, want completed", finished)
	}
	mainCalls, summaryCalls, staleSeen, currentSeen := model.Snapshot()
	if mainCalls != modelCallsBeforeMidRunCompaction+1 || summaryCalls != 1 || staleSeen || !currentSeen {
		t.Fatalf(
			"post-compaction model context = main:%d summary:%d stale:%t current:%t",
			mainCalls,
			summaryCalls,
			staleSeen,
			currentSeen,
		)
	}
	currentPlan, err := api.GetPlan(ctx, protocol.GetPlanRequest{SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	if len(currentPlan.Steps) != 1 || currentPlan.Steps[0].Description != currentPlanText {
		t.Fatalf("durable Plan = %+v, want current replacement", currentPlan)
	}
}

func TestActiveGoalAndPlanStayCurrentAcrossLongRunCompaction(t *testing.T) {
	model := newGoalAndPlanLongContextModel()
	defer model.releaseOpeningCall()
	stores, api, ctx, home := newSessionStateE2ERuntime(t, model)
	sessionID := createSessionWithInitialPlan(t, stores, api, ctx, home, "Goal and Plan compaction ownership")
	startedGoal, err := api.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: sessionID,
		Objective: goalAcrossCompactionObjective,
		Provider:  "anthropic",
		Model:     "claude-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if startedGoal.Status != protocol.GoalActive || startedGoal.Objective != goalAcrossCompactionObjective {
		t.Fatalf("started Goal = %+v", startedGoal)
	}
	waitForSignal(t, model.openingCallArrived, "Goal-owned opening model call")
	durableGoal, found, err := stores.Goals.Get(ctx, sessionID)
	if err != nil || !found {
		t.Fatalf("durable active Goal found=%t error=%v", found, err)
	}
	if durableGoal.Objective != goalAcrossCompactionObjective ||
		string(durableGoal.Status) != string(protocol.GoalActive) ||
		durableGoal.IncarnationID == "" {
		t.Fatalf("durable active Goal = %+v", durableGoal)
	}
	model.releaseOpeningCall()

	deadline := time.Now().Add(goalSettlementTimeout)
	for {
		current, getErr := api.GetGoal(ctx, protocol.GoalRequest{SessionID: sessionID})
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Goal did not settle and clear before %s: %+v", goalSettlementTimeout, current)
		}
		time.Sleep(goalSettlementPollInterval)
	}
	mainCalls, summaryCalls, summaryAtMainCalls, stateChecks := model.Snapshot()
	if mainCalls != modelCallsBeforeMidRunCompaction+2 ||
		summaryCalls != 1 || len(summaryAtMainCalls) != 1 ||
		summaryAtMainCalls[0] != modelCallsBeforeMidRunCompaction || !stateChecks.all() {
		t.Fatalf(
			"Goal/Plan model context = main:%d summary:%d boundary:%v checks:%+v",
			mainCalls,
			summaryCalls,
			summaryAtMainCalls,
			stateChecks,
		)
	}
	planState, err := stores.Plan.State(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	steps := planState.Steps()
	if planState.Revision() != 2 || len(steps) != 1 || steps[0].Description != currentPlanText {
		t.Fatalf("durable Plan state = revision:%d steps:%+v", planState.Revision(), steps)
	}
}

type sessionStateE2EModel interface {
	chat.Model
	chat.Streamer
}

func newSessionStateE2ERuntime(
	t *testing.T,
	model sessionStateE2EModel,
) (*persistence.Bundle, *runtimeserver.Server, context.Context, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("SCOPEAPP_HOME", home)
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: home, DefaultWorkspacePath: home,
	})
	if err != nil {
		t.Fatal(err)
	}
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
	t.Cleanup(func() {
		api.Close()
		if closeErr := host.Close(); closeErr != nil {
			t.Errorf("close runtime: %v", closeErr)
		}
	})
	ctx := operation.WithRequestMeta(t.Context(), protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
	})
	return stores, api, ctx, home
}

func createSessionWithInitialPlan(
	t *testing.T,
	stores *persistence.Bundle,
	api *runtimeserver.Server,
	ctx context.Context,
	home string,
	title string,
) string {
	t.Helper()
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: title,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planapp.New(planapp.Dependencies{Store: stores.Plan}).Replace(
		ctx,
		session.ID,
		[]plandomain.Step{{Description: stalePlanText, Status: plandomain.StatusInProgress}},
	); err != nil {
		t.Fatal(err)
	}
	return session.ID
}

const (
	goalAcrossCompactionObjective = "preserve the exact active Goal while replacing the Plan across compaction"
	goalSettlementPollInterval    = 10 * time.Millisecond
	goalSettlementTimeout         = 5 * time.Second
	stalePlanText                 = "stale frozen Plan"
	currentPlanText               = "current durable Plan"
)

type goalAndPlanStateChecks struct {
	openingGoalAndPlan bool
	updatedBeforeFold  bool
	updatedAfterFold   bool
	completingGoal     bool
}

func (g goalAndPlanStateChecks) all() bool {
	return g.openingGoalAndPlan && g.updatedBeforeFold && g.updatedAfterFold && g.completingGoal
}

type goalAndPlanLongContextModel struct {
	mu                     sync.Mutex
	mainCalls              int
	summaryCalls           int
	summaryAtMainCalls     []int
	checks                 goalAndPlanStateChecks
	openingCallArrived     chan struct{}
	openingCallArrivedOnce sync.Once
	openingCallRelease     chan struct{}
	openingCallReleaseOnce sync.Once
}

func newGoalAndPlanLongContextModel() *goalAndPlanLongContextModel {
	return &goalAndPlanLongContextModel{
		openingCallArrived: make(chan struct{}),
		openingCallRelease: make(chan struct{}),
	}
}

func (g *goalAndPlanLongContextModel) Call(
	ctx context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	g.mu.Lock()
	if isCompactionRequest(request) {
		g.summaryCalls++
		g.summaryAtMainCalls = append(g.summaryAtMainCalls, g.mainCalls)
		g.mu.Unlock()
		// Deliberately omit Goal and Plan from the model-authored summary. Their
		// exact current values must come from the replaceable Session-state owner.
		return completedTextResponse("## Progress\nThe long autonomous Tool loop reached its context boundary."), nil
	}
	g.mainCalls++
	call := g.mainCalls
	g.mu.Unlock()

	switch {
	case call == 1:
		g.openingCallArrivedOnce.Do(func() { close(g.openingCallArrived) })
		select {
		case <-g.openingCallRelease:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		g.recordCheck(func(checks *goalAndPlanStateChecks) {
			checks.openingGoalAndPlan = requestContainsText(request, goalAcrossCompactionObjective) &&
				requestContainsText(request, "Status: active") &&
				requestContainsText(request, stalePlanText)
		})
		return toolCallResponse(chat.ToolCall{
			ID:        "call_goal_set_current_plan",
			Name:      tool.SetPlan,
			Arguments: `{"steps":[{"description":"` + currentPlanText + `","status":"in_progress"}]}`,
		}), nil
	case call <= modelCallsBeforeMidRunCompaction:
		if call == 2 {
			g.recordCheck(func(checks *goalAndPlanStateChecks) {
				checks.updatedBeforeFold = requestContainsText(request, goalAcrossCompactionObjective) &&
					!requestContainsText(request, stalePlanText) &&
					requestContainsText(request, currentPlanText)
			})
		}
		return toolCallResponse(chat.ToolCall{
			ID: fmt.Sprintf("call_active_goal_%02d", call), Name: tool.GetGoal, Arguments: `{}`,
		}), nil
	case call == modelCallsBeforeMidRunCompaction+1:
		g.recordCheck(func(checks *goalAndPlanStateChecks) {
			checks.updatedAfterFold = requestContainsText(request, goalAcrossCompactionObjective) &&
				requestContainsText(request, "Status: active") &&
				!requestContainsText(request, stalePlanText) &&
				requestContainsText(request, currentPlanText)
		})
		return toolCallResponse(chat.ToolCall{
			ID: "call_report_compacted_goal", Name: tool.ReportGoalOutcome, Arguments: `{"outcome":"completed"}`,
		}), nil
	default:
		g.recordCheck(func(checks *goalAndPlanStateChecks) {
			checks.completingGoal = requestContainsText(request, goalAcrossCompactionObjective) &&
				requestContainsText(request, "Status: complete") &&
				requestContainsText(request, currentPlanText)
		})
		return completedTextResponse("The autonomous Goal is complete after compaction."), nil
	}
}

func (g *goalAndPlanLongContextModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		response, err := g.Call(ctx, request)
		yield(response, err)
	}
}

func (g *goalAndPlanLongContextModel) recordCheck(update func(*goalAndPlanStateChecks)) {
	g.mu.Lock()
	update(&g.checks)
	g.mu.Unlock()
}

func (g *goalAndPlanLongContextModel) releaseOpeningCall() {
	g.openingCallReleaseOnce.Do(func() { close(g.openingCallRelease) })
}

func (g *goalAndPlanLongContextModel) Snapshot() (
	mainCalls int,
	summaryCalls int,
	summaryAtMainCalls []int,
	checks goalAndPlanStateChecks,
) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mainCalls, g.summaryCalls, append([]int(nil), g.summaryAtMainCalls...), g.checks
}

type planChangingLongContextModel struct {
	mu           sync.Mutex
	mainCalls    int
	summaryCalls int
	staleSeen    bool
	currentSeen  bool
}

func (p *planChangingLongContextModel) Call(
	_ context.Context,
	request *chat.Request,
) (*chat.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if isCompactionRequest(request) {
		p.summaryCalls++
		return completedTextResponse("## Progress\nThe active Tool loop reached its context boundary."), nil
	}
	p.mainCalls++
	switch {
	case p.mainCalls == 1:
		if !requestContainsText(request, stalePlanText) {
			return nil, fmt.Errorf("opening model context does not contain the seeded Plan")
		}
		return toolCallResponse(chat.ToolCall{
			ID:        "call_set_current_plan",
			Name:      tool.SetPlan,
			Arguments: `{"steps":[{"description":"` + currentPlanText + `","status":"in_progress"}]}`,
		}), nil
	case p.mainCalls <= modelCallsBeforeMidRunCompaction:
		if p.mainCalls == 2 &&
			(requestContainsText(request, stalePlanText) || !requestContainsText(request, currentPlanText)) {
			return nil, fmt.Errorf("second model call did not replace the opening Plan snapshot")
		}
		return toolCallResponse(chat.ToolCall{
			ID: fmt.Sprintf("call_goal_after_plan_%02d", p.mainCalls), Name: tool.GetGoal, Arguments: `{}`,
		}), nil
	default:
		p.staleSeen = requestContainsText(request, stalePlanText)
		p.currentSeen = requestContainsText(request, currentPlanText)
		return completedTextResponse("Plan remained current across compaction."), nil
	}
}

func (p *planChangingLongContextModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		response, err := p.Call(ctx, request)
		yield(response, err)
	}
}

func (p *planChangingLongContextModel) Snapshot() (
	mainCalls int,
	summaryCalls int,
	staleSeen bool,
	currentSeen bool,
) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.mainCalls, p.summaryCalls, p.staleSeen, p.currentSeen
}

func requestContainsText(request *chat.Request, text string) bool {
	for _, message := range request.Messages {
		if strings.Contains(message.Text(), text) {
			return true
		}
	}
	return false
}

func toolCallResponse(call chat.ToolCall) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	return &chat.Response{Output: &chat.Output{
		Message: &message, FinishReason: chat.FinishReasonToolCalls,
	}}
}
