package event

// JSON marshaling for every event type lives here, separate from the
// type definitions in process.go / execution.go / planning.go /
// platform.go. The split is an SRP move: event definitions describe
// what the runtime publishes; marshaling describes how it serializes
// onto an external wire (audit log, websocket fan-out, etc).
//
// Listeners that only care about in-memory dispatch can ignore this
// file entirely — they type-assert the concrete event types and read
// their public fields directly. Marshaling here is best-effort lossy
// JSON (interface-typed fields like core.Action / planning.Plan
// collapse to summaries; see summaries.go for the helpers).

// ------------------------------------------------------------------
// platform events
// ------------------------------------------------------------------

func (e AgentDeployed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
}

func (e AgentUndeployed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"agent_name": e.AgentName})
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
	return emit(e, map[string]any{"world": snapshotWorld(e.LastWorld)})
}

func (e ProcessWaiting) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"awaitable": summarizeAwaitable(e.Awaitable)})
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

func (e ReadyToPlan) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.World)})
}

func (e PlanFormulated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"plan": summarizePlan(e.Plan)})
}

func (e ReplanRequested) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": e.Action, "reason": e.Reason})
}

// ------------------------------------------------------------------
// action execution
// ------------------------------------------------------------------

func (e ActionExecutionStart) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": actionName(e.Action), "started_at": e.StartedAt})
}

func (e ActionExecutionResult) MarshalJSON() ([]byte, error) {
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
// LLM / embedding invocations
// ------------------------------------------------------------------

func (e LLMInvocationRecorded) MarshalJSON() ([]byte, error) {
	inv := e.Invocation
	return emit(e, map[string]any{
		"model":             inv.Model,
		"provider":          inv.Provider,
		"cost":              inv.CostUSD,
		"prompt_tokens":     inv.PromptTokens,
		"completion_tokens": inv.CompletionTokens,
		"reasoning_tokens":  inv.ReasoningTokens,
		"duration_ns":       inv.Duration.Nanoseconds(),
		"action":            inv.Action,
	})
}

func (e EmbeddingInvocationRecorded) MarshalJSON() ([]byte, error) {
	inv := e.Invocation
	return emit(e, map[string]any{
		"model":        inv.Model,
		"provider":     inv.Provider,
		"cost":         inv.CostUSD,
		"input_tokens": inv.InputTokens,
		"input_count":  inv.InputCount,
		"duration_ns":  inv.Duration.Nanoseconds(),
		"action":       inv.Action,
	})
}
