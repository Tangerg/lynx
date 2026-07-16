package event

// JSON marshaling for every event type lives here, separate from the
// type definitions in process.go / execution.go / planning.go /
// engine.go. The split is an SRP move: event definitions describe
// what the runtime publishes; marshaling describes how it serializes
// onto an external wire (audit log, websocket fan-out, etc).
//
// Listeners that only care about in-memory dispatch can ignore this
// file entirely — they type-assert the concrete event types and read
// their public fields directly. Marshaling here is best-effort lossy
// JSON (interface-typed fields like core.Action / planning.Plan
// collapse to summaries; see summaries.go for the helpers).

// ------------------------------------------------------------------
// engine events
// ------------------------------------------------------------------

func (e AgentDeployed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"deployment": e.Deployment})
}

func (e AgentUndeployed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"deployment": e.Deployment})
}

// ------------------------------------------------------------------
// process lifecycle
// ------------------------------------------------------------------

func (e ProcessCreated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"bindings": summarizeMap(e.Bindings)})
}

func (e ProcessCompleted) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal), "result": summarizeValue(e.Result)})
}

func (e ProcessFailed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"error": errString(e.Err)})
}

func (e ProcessStuck) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"state": summarizeWorldState(e.State)})
}

func (e ProcessWaiting) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"suspension": e.Suspension})
}

func (e InteractionBoundary) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"deployment":     e.Deployment,
		"interaction_id": e.InteractionID,
		"boundary":       e.Boundary,
	})
}

func (e ProcessKilled) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason})
}

func (e ProcessTerminated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason, "scope": e.Scope.String()})
}

// ------------------------------------------------------------------
// planning
// ------------------------------------------------------------------

func (e PlanningStarted) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"state": summarizeWorldState(e.State)})
}

func (e PlanCreated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"plan": summarizePlan(e.Plan)})
}

func (e ReplanRequested) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": e.ActionName, "reason": e.Reason})
}

// ------------------------------------------------------------------
// action execution
// ------------------------------------------------------------------

func (e ActionStarted) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": actionName(e.Action), "started_at": e.StartedAt})
}

func (e ActionFinished) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"action":      actionName(e.Action),
		"status":      e.Status.String(),
		"duration_ns": e.Duration.Nanoseconds(),
		"error":       errString(e.Err),
	})
}

func (e GoalAchieved) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal)})
}

// ------------------------------------------------------------------
// Model and embedding calls
// ------------------------------------------------------------------

func (e ModelCallRecorded) MarshalJSON() ([]byte, error) {
	call := e.Call
	return emit(e, map[string]any{
		"model":             call.Model,
		"provider":          call.Provider,
		"cost":              call.CostUSD,
		"prompt_tokens":     call.PromptTokens,
		"completion_tokens": call.CompletionTokens,
		"reasoning_tokens":  call.ReasoningTokens,
		"duration_ns":       call.Duration.Nanoseconds(),
		"action":            call.ActionName,
	})
}

func (e EmbeddingCallRecorded) MarshalJSON() ([]byte, error) {
	call := e.Call
	return emit(e, map[string]any{
		"model":        call.Model,
		"provider":     call.Provider,
		"cost":         call.CostUSD,
		"input_tokens": call.InputTokens,
		"input_count":  call.InputCount,
		"duration_ns":  call.Duration.Nanoseconds(),
		"action":       call.ActionName,
	})
}
