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

var errChildToolManifestChanged = errors.New("agentexec: child Tool manifest changed")

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
	result, err := executable.Call(ctx, arguments)
	if err != nil {
		return "", err
	}
	return tool.router.observer.boundToolResult(ctx, result)
}

type runToolRouter struct {
	mu       sync.Mutex
	catalog  ToolCatalog
	bridge   *delegationBridge
	base     ToolScope
	observer *executionObserver
	manifest map[string]toolBindingContract
	byRunID  map[string]*toolResolution
}

type toolResolution struct {
	ready chan struct{}
	tools map[string]toolcontract.Tool
	err   error
}

type toolBindingContract struct {
	definition     chat.ToolDefinition
	safety         protocol.SafetyClass
	deferred       bool
	intrinsicInput bool
}

func newRunToolRouter(
	catalog ToolCatalog,
	bridge *delegationBridge,
	base ToolScope,
	observer *executionObserver,
	bindings []ExecutableTool,
) (*runToolRouter, error) {
	manifest := make(map[string]toolBindingContract, len(bindings))
	for _, binding := range bindings {
		definition := binding.Tool.Definition()
		if _, duplicate := manifest[definition.Name]; duplicate {
			return nil, fmt.Errorf("agentexec: duplicate child Tool %q", definition.Name)
		}
		manifest[definition.Name] = toolBindingContract{
			definition: definition.Clone(), safety: binding.SafetyClass,
			deferred: binding.Deferred, intrinsicInput: binding.IntrinsicInput,
		}
	}
	return &runToolRouter{
		catalog: catalog, bridge: bridge, base: base, observer: observer,
		manifest: manifest,
		byRunID:  make(map[string]*toolResolution),
	}, nil
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
	resolution := router.byRunID[binding.runID]
	owner := resolution == nil
	if owner {
		resolution = &toolResolution{ready: make(chan struct{})}
		router.byRunID[binding.runID] = resolution
	}
	router.mu.Unlock()
	if !owner {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-resolution.ready:
			return resolvedTool(resolution, want)
		}
	}

	scope := router.base
	scope.RunID = binding.runID
	scope.IsRootRun = false
	scope.Facts = scopedToolFacts{runID: binding.runID, observer: router.observer}
	resolved, err := router.resolveManifest(ctx, scope)
	router.mu.Lock()
	resolution.tools = resolved
	resolution.err = err
	close(resolution.ready)
	if err != nil && !errors.Is(err, errChildToolManifestChanged) &&
		router.byRunID[binding.runID] == resolution {
		delete(router.byRunID, binding.runID)
	}
	router.mu.Unlock()
	return resolvedTool(resolution, want)
}

func (router *runToolRouter) resolveManifest(
	ctx context.Context,
	scope ToolScope,
) (map[string]toolcontract.Tool, error) {
	bindings, err := router.catalog.ForRun(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("agentexec: resolve child Tools: %w", err)
	}
	if len(bindings) != len(router.manifest) {
		return nil, errChildToolManifestChanged
	}
	resolved := make(map[string]toolcontract.Tool, len(bindings))
	for _, binding := range bindings {
		definition := binding.Tool.Definition()
		contract, expected := router.manifest[definition.Name]
		if !expected || !reflect.DeepEqual(definition, contract.definition) ||
			binding.SafetyClass != contract.safety ||
			binding.Deferred != contract.deferred ||
			binding.IntrinsicInput != contract.intrinsicInput {
			return nil, fmt.Errorf("%w for %q", errChildToolManifestChanged, definition.Name)
		}
		if _, duplicate := resolved[definition.Name]; duplicate {
			return nil, fmt.Errorf("agentexec: duplicate resolved child Tool %q", definition.Name)
		}
		resolved[definition.Name] = binding.Tool
	}
	return resolved, nil
}

func resolvedTool(
	resolution *toolResolution,
	want chat.ToolDefinition,
) (toolcontract.Tool, error) {
	if resolution.err != nil {
		return nil, resolution.err
	}
	executable := resolution.tools[want.Name]
	if executable == nil || !reflect.DeepEqual(executable.Definition(), want) {
		return nil, fmt.Errorf("%w for %q", errChildToolManifestChanged, want.Name)
	}
	return executable, nil
}

func routedManifest(bindings []ExecutableTool, router *runToolRouter) []ExecutableTool {
	result := make([]ExecutableTool, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, ExecutableTool{
			Tool: &routedTool{
				definition: binding.Tool.Definition(), template: binding.Tool, router: router,
			},
			SafetyClass: binding.SafetyClass, Deferred: binding.Deferred,
			IntrinsicInput: binding.IntrinsicInput,
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

func (facts scopedToolFacts) RecordEffectiveToolArguments(callID string, arguments map[string]any) {
	facts.observer.recordEffectiveToolArgumentsFor(facts.runID, callID, arguments)
}
