package event

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/planning"
)

// JSON marshaling for every event type lives here, separate from the
// type definitions in process.go / action.go / planning.go /
// deployment.go. The split is an SRP move: event definitions describe
// what the runtime publishes; marshaling describes how it serializes
// onto an external wire (audit log, websocket fan-out, etc).
//
// Listeners that only care about in-memory dispatch can ignore this
// file entirely — they type-assert the concrete event types and read
// their public fields directly. Marshaling here is best-effort lossy
// JSON (interface-typed fields like core.Action / planning.Plan
// collapse to summaries; see json_summary.go for the helpers).

// ------------------------------------------------------------------
// engine events
// ------------------------------------------------------------------

type deploymentPayload struct {
	Deployment core.DeploymentRef `json:"deployment"`
}

func (e AgentDeployed) MarshalJSON() ([]byte, error) {
	return emit(e, deploymentPayload{Deployment: e.Deployment})
}

func (e AgentUndeployed) MarshalJSON() ([]byte, error) {
	return emit(e, deploymentPayload{Deployment: e.Deployment})
}

// ------------------------------------------------------------------
// process lifecycle
// ------------------------------------------------------------------

type processCreatedPayload struct {
	Bindings map[string]any `json:"bindings"`
}

func (e ProcessCreated) MarshalJSON() ([]byte, error) {
	return emit(e, processCreatedPayload{Bindings: summarizeBindings(e.Bindings)})
}

type processCompletedPayload struct {
	Goal   *goalSummary `json:"goal"`
	Result any          `json:"result"`
}

func (e ProcessCompleted) MarshalJSON() ([]byte, error) {
	return emit(e, processCompletedPayload{
		Goal:   summarizeGoal(e.Goal),
		Result: summarizeValue(e.Result),
	})
}

type errorPayload struct {
	Error string `json:"error"`
}

func (e ProcessFailed) MarshalJSON() ([]byte, error) {
	return emit(e, errorPayload{Error: errString(e.Err)})
}

type worldStatePayload struct {
	State *worldStateSummary `json:"state"`
}

func (e ProcessStuck) MarshalJSON() ([]byte, error) {
	return emit(e, worldStatePayload{State: summarizeWorldState(e.State)})
}

type processWaitingPayload struct {
	Suspension *interaction.Suspension `json:"suspension"`
}

func (e ProcessWaiting) MarshalJSON() ([]byte, error) {
	return emit(e, processWaitingPayload{Suspension: e.Suspension})
}

type processSnapshotFailedPayload struct {
	Policy string `json:"policy"`
	Error  string `json:"error"`
}

func (e ProcessSnapshotFailed) MarshalJSON() ([]byte, error) {
	return emit(e, processSnapshotFailedPayload{
		Policy: e.Policy,
		Error:  errString(e.Err),
	})
}

type interactionBoundaryPayload struct {
	Deployment    core.DeploymentRef `json:"deployment"`
	InteractionID string             `json:"interaction_id"`
	Boundary      interaction.Event  `json:"boundary"`
}

func (e InteractionBoundary) MarshalJSON() ([]byte, error) {
	return emit(e, interactionBoundaryPayload{
		Deployment:    e.Deployment,
		InteractionID: e.InteractionID,
		Boundary:      e.Boundary,
	})
}

type reasonPayload struct {
	Reason string `json:"reason"`
}

func (e ProcessKilled) MarshalJSON() ([]byte, error) {
	return emit(e, reasonPayload{Reason: e.Reason})
}

type processTerminatedPayload struct {
	Reason string `json:"reason"`
	Scope  string `json:"scope"`
}

func (e ProcessTerminated) MarshalJSON() ([]byte, error) {
	return emit(e, processTerminatedPayload{
		Reason: e.Reason,
		Scope:  e.Scope.String(),
	})
}

// ------------------------------------------------------------------
// planning
// ------------------------------------------------------------------

func (e PlanningStarted) MarshalJSON() ([]byte, error) {
	return emit(e, worldStatePayload{State: summarizeWorldState(e.State)})
}

type planCreatedPayload struct {
	Plan *planSummary `json:"plan"`
}

func (e PlanCreated) MarshalJSON() ([]byte, error) {
	return emit(e, planCreatedPayload{Plan: summarizePlan(e.Plan)})
}

type replanRequestedPayload struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (e ReplanRequested) MarshalJSON() ([]byte, error) {
	return emit(e, replanRequestedPayload{
		Action: e.ActionName,
		Reason: e.Reason,
	})
}

// ------------------------------------------------------------------
// action execution
// ------------------------------------------------------------------

type actionStartedPayload struct {
	Action    string    `json:"action"`
	StartedAt time.Time `json:"started_at"`
}

func (e ActionStarted) MarshalJSON() ([]byte, error) {
	return emit(e, actionStartedPayload{
		Action:    actionName(e.Action),
		StartedAt: e.StartedAt,
	})
}

type actionFinishedPayload struct {
	Action     string `json:"action"`
	Status     string `json:"status"`
	DurationNS int64  `json:"duration_ns"`
	Error      string `json:"error"`
}

func (e ActionFinished) MarshalJSON() ([]byte, error) {
	return emit(e, actionFinishedPayload{
		Action:     actionName(e.Action),
		Status:     e.Status.String(),
		DurationNS: e.Duration.Nanoseconds(),
		Error:      errString(e.Err),
	})
}

type goalPayload struct {
	Goal *goalSummary `json:"goal"`
}

func (e GoalAchieved) MarshalJSON() ([]byte, error) {
	return emit(e, goalPayload{Goal: summarizeGoal(e.Goal)})
}

// ------------------------------------------------------------------
// Model and embedding calls
// ------------------------------------------------------------------

type modelCallPayload struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	Cost             float64 `json:"cost"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	ReasoningTokens  int64   `json:"reasoning_tokens"`
	DurationNS       int64   `json:"duration_ns"`
	Action           string  `json:"action"`
}

func (e ModelCallRecorded) MarshalJSON() ([]byte, error) {
	call := e.Call
	return emit(e, modelCallPayload{
		Model:            call.Model,
		Provider:         call.Provider,
		Cost:             call.CostUSD,
		PromptTokens:     call.PromptTokens,
		CompletionTokens: call.CompletionTokens,
		ReasoningTokens:  call.ReasoningTokens,
		DurationNS:       call.Duration.Nanoseconds(),
		Action:           call.ActionName,
	})
}

type embeddingCallPayload struct {
	Model       string  `json:"model"`
	Provider    string  `json:"provider"`
	Cost        float64 `json:"cost"`
	InputTokens int64   `json:"input_tokens"`
	InputCount  int     `json:"input_count"`
	DurationNS  int64   `json:"duration_ns"`
	Action      string  `json:"action"`
}

func (e EmbeddingCallRecorded) MarshalJSON() ([]byte, error) {
	call := e.Call
	return emit(e, embeddingCallPayload{
		Model:       call.Model,
		Provider:    call.Provider,
		Cost:        call.CostUSD,
		InputTokens: call.InputTokens,
		InputCount:  call.InputCount,
		DurationNS:  call.Duration.Nanoseconds(),
		Action:      call.ActionName,
	})
}

// goalSummary is the lossy wire representation of a Goal. Function-valued
// planning callbacks intentionally do not cross the event boundary.
type goalSummary struct {
	Name          string         `json:"name,omitempty"`
	Description   string         `json:"description,omitempty"`
	Preconditions []string       `json:"pre,omitempty"`
	Inputs        []core.Binding `json:"inputs,omitempty"`
}

func summarizeGoal(goal *core.Goal) *goalSummary {
	if goal == nil {
		return nil
	}
	return &goalSummary{
		Name:          goal.Name(),
		Description:   goal.Description(),
		Preconditions: goal.RequiredConditions(),
		Inputs:        goal.Inputs(),
	}
}

func actionName(action core.Action) string {
	if action == nil {
		return ""
	}
	return action.Metadata().Name
}

func actionNames(actions []core.Action) []string {
	if len(actions) == 0 {
		return nil
	}
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, actionName(action))
	}
	return names
}

type planSummary struct {
	Actions []string     `json:"actions,omitempty"`
	Goal    *goalSummary `json:"goal,omitempty"`
}

func summarizePlan(plan *planning.Plan) *planSummary {
	if plan == nil {
		return nil
	}
	return &planSummary{
		Actions: actionNames(plan.Actions()),
		Goal:    summarizeGoal(plan.Goal()),
	}
}

type worldStateSummary struct {
	State     map[string]core.Truth `json:"state,omitempty"`
	Timestamp time.Time             `json:"timestamp,omitzero"`
}

func summarizeWorldState(state core.WorldState) *worldStateSummary {
	if state == nil {
		return nil
	}
	return &worldStateSummary{State: state.Conditions(), Timestamp: state.Timestamp()}
}

func summarizeValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return fmt.Sprint(value)
	}
	return decoded
}

func summarizeBindings(bindings core.Bindings) map[string]any {
	if bindings.Len() == 0 {
		return nil
	}
	summary := make(map[string]any, bindings.Len())
	for key, value := range bindings.All() {
		summary[key] = summarizeValue(value)
	}
	return summary
}
