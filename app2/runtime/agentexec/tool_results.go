package agentexec

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/interaction"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
)

// boundedResultTool keeps a Tool's complete result inside the Runtime while
// returning only the canonical preview to the active model loop. The observer
// restores the full body into its semantic ToolObservation, so persistence and
// presentation do not depend on provider-facing context truncation.
type boundedResultTool struct {
	toolcontract.Tool
	observer *executionObserver
}

func (tool *boundedResultTool) Unwrap() toolcontract.Tool { return tool.Tool }

func (tool *boundedResultTool) Call(ctx context.Context, arguments string) (string, error) {
	result, err := tool.Tool.Call(ctx, arguments)
	if err != nil {
		return "", err
	}
	return tool.observer.boundToolResult(ctx, result)
}

func boundToolResults(values []ExecutableTool, observer *executionObserver) []ExecutableTool {
	bounded := make([]ExecutableTool, len(values))
	for index, value := range values {
		bounded[index] = value
		bounded[index].Tool = &boundedResultTool{Tool: value.Tool, observer: observer}
	}
	return bounded
}

func (observer *executionObserver) boundToolResult(ctx context.Context, body string) (string, error) {
	if !toolresult.NeedsOffload(body) {
		return body, nil
	}
	invocation, found := interaction.ToolInvocationFromContext(ctx)
	if !found {
		return "", errors.New("agentexec: large Tool result has no Interaction identity")
	}
	scope, found := observer.scope(invocation.Relation())
	call := invocation.ToolCall()
	if !found || scope.runID == "" || call.ID == "" {
		return "", errors.New("agentexec: large Tool result has no product Run identity")
	}
	identity := observationIdentity{runID: scope.runID, sourceID: call.ID}
	projected := toolresult.Project(ToolItemID(scope.runID, call.ID), body)
	observer.mu.Lock()
	if previous, duplicate := observer.fullToolResults[identity]; duplicate && previous != body {
		observer.mu.Unlock()
		return "", errors.New("agentexec: ToolCall produced conflicting large results")
	}
	observer.fullToolResults[identity] = body
	observer.mu.Unlock()
	return projected.Preview, nil
}
