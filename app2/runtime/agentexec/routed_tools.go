package agentexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

// routedTool preserves a frozen model-visible definition while resolving the
// actual Run-scoped capability from the current child invocation. Its template
// is used only for immutable optional capabilities; it is never called.
type routedTool struct {
	definition chat.ToolDefinition
	template   toolcontract.Tool
	router     *runToolRouter
}

func (tool *routedTool) Definition() chat.ToolDefinition { return tool.definition.Clone() }
func (tool *routedTool) Unwrap() toolcontract.Tool       { return tool.template }

func (tool *routedTool) Call(ctx context.Context, arguments string) (string, error) {
	invocation, found := interaction.ToolInvocationFromContext(ctx)
	if !found {
		return "", errors.New("agentexec: routed Tool called outside an Interaction")
	}
	executable, err := tool.router.resolve(ctx, invocation, tool.definition)
	if err != nil {
		return "", err
	}
	return executable.Call(ctx, arguments)
}

type runToolRouter struct {
	mu        sync.Mutex
	catalog   ToolCatalog
	bridge    *delegationBridge
	base      ToolScope
	observer  *executionObserver
	byRunID   map[string]map[string]toolcontract.Tool
}

func newRunToolRouter(catalog ToolCatalog, bridge *delegationBridge, base ToolScope, observer *executionObserver) *runToolRouter {
	return &runToolRouter{
		catalog: catalog, bridge: bridge, base: base, observer: observer,
		byRunID: make(map[string]map[string]toolcontract.Tool),
	}
}

func (router *runToolRouter) resolve(
	ctx context.Context,
	invocation interaction.ToolInvocation,
	want chat.ToolDefinition,
) (toolcontract.Tool, error) {
	binding, found := router.bridge.binding(invocation.Relation())
	if !found || binding.runID == "" {
		return nil, errors.New("agentexec: child Tool has no product Run binding")
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	if executable := router.byRunID[binding.runID][want.Name]; executable != nil {
		return executable, nil
	}
	scope := router.base
	scope.RunID = binding.runID
	scope.IsRootRun = false
	scope.Facts = scopedToolFacts{runID: binding.runID, observer: router.observer}
	bindings, err := router.catalog.ForRun(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("agentexec: resolve child Tools: %w", err)
	}
	resolved := make(map[string]toolcontract.Tool, len(bindings))
	for _, binding := range bindings {
		definition := binding.Tool.Definition()
		resolved[definition.Name] = binding.Tool
	}
	executable := resolved[want.Name]
	if executable == nil || !reflect.DeepEqual(executable.Definition(), want) {
		return nil, fmt.Errorf("agentexec: child Tool manifest changed for %q", want.Name)
	}
	router.byRunID[binding.runID] = resolved
	return executable, nil
}

func routedManifest(bindings []ExecutableTool, router *runToolRouter) []toolcontract.Tool {
	result := make([]toolcontract.Tool, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, &routedTool{
			definition: binding.Tool.Definition(), template: binding.Tool, router: router,
		})
	}
	return result
}

type scopedToolFacts struct {
	runID    string
	observer *executionObserver
}

func (facts scopedToolFacts) RecordCommittedPlan(callID string, plan protocol.Plan) {
	facts.observer.recordCommittedPlanFor(facts.runID, callID, plan)
}
