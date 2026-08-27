package runs

import (
	"strings"

	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	corechat "github.com/Tangerg/scope/core/chat"
)

// appendToolContext advances the reducer-local model-context ledger only for a
// root Run. Child model context belongs to its executor; Runtime's durable
// Session conversation is the root projection.
func (r *reducer) appendToolContext(messages []corechat.Message) error {
	if !r.cfg.Lineage.IsRoot() || len(messages) == 0 {
		return nil
	}
	next, err := r.toolContext.Append(messages...)
	if err != nil {
		return err
	}
	r.toolContext = next
	return nil
}

// trackStartedToolCall covers resumed calls whose assistant ToolCall was
// committed by an earlier Segment. Repeating an already-open provider call is
// harmless: Conversation.CloseOpenToolCalls treats it as the same generation.
func (r *reducer) trackStartedToolCall(started ToolCallStarted) error {
	if !r.cfg.Lineage.IsRoot() || started.ModelCallSequence == 0 {
		return nil
	}
	return r.appendToolContext([]corechat.Message{corechat.NewAssistantMessage(
		corechat.NewToolCallPart(corechat.ToolCall{
			ID: started.SourceCallID, Name: started.ToolName, Arguments: started.Arguments,
		}),
	)})
}

// trackUnconsumedResumeToolCalls covers cancellation before the resumed
// executor can re-announce its suspended tools. Those calls are present in the
// durable conversation and continuation, so terminalization must close them.
func (r *reducer) trackUnconsumedResumeToolCalls() error {
	if !r.cfg.Lineage.IsRoot() || r.resume == nil {
		return nil
	}
	var parts []corechat.Part
	for _, drained := range r.resume.remainingDrainedTools() {
		if drained.SourceCallID == "" {
			continue
		}
		parts = append(parts, corechat.NewToolCallPart(corechat.ToolCall{
			ID: drained.SourceCallID, Name: drained.Name, Arguments: drained.Arguments,
		}))
	}
	if len(parts) == 0 {
		return nil
	}
	return r.appendToolContext([]corechat.Message{corechat.NewAssistantMessage(parts...)})
}

func (r *reducer) closeOpenToolContext(
	result string,
	completed []corechat.ToolResult,
) ([]corechat.Message, error) {
	if !r.cfg.Lineage.IsRoot() {
		return nil, nil
	}
	closed, appended, err := r.toolContext.CloseOpenToolCallsWithResults(result, completed)
	if err != nil {
		return nil, err
	}
	r.toolContext = closed
	return appended, nil
}

func completedTerminalToolResults(open []*openTool) []corechat.ToolResult {
	results := make([]corechat.ToolResult, 0, len(open))
	for _, ref := range open {
		if ref == nil || ref.modelCallSequence == 0 || ref.end == nil {
			continue
		}
		results = append(results, conversationToolResult(ref, *ref.end))
	}
	return results
}

func (r *reducer) cancelReason() string {
	if r.cfg.CancelReason == nil {
		return ""
	}
	return r.cfg.CancelReason()
}

func terminalToolResult(outcome run.Outcome, detail string) string {
	var result string
	switch outcome {
	case run.OutcomeCanceled:
		result = "tool call canceled before completion"
	case run.OutcomeTimedOut:
		result = "tool call did not complete before the run timed out"
	case run.OutcomeLost:
		result = "tool result unavailable because execution state was lost"
	case run.OutcomeFailed:
		result = "tool call did not complete because the run failed"
	case run.OutcomeMaxBudget:
		result = "tool call did not complete before the run reached its budget"
	default:
		result = "tool call ended without a result before the run finished"
	}
	if detail = strings.TrimSpace(detail); detail != "" && outcome == run.OutcomeCanceled {
		result += ": " + detail
	}
	return result
}
