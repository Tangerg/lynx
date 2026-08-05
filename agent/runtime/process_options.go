package runtime

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
)

// processOptions is the runtime-owned subset of [core.ProcessOptions]. The
// public struct is a construction DTO; retaining it directly would let caller
// slice, pointer, and map mutations rewrite a live Process after construction.
// Blackboard and Dependencies are prepared into dedicated Process fields and
// therefore do not appear here.
type processOptions struct {
	configureChild core.ConfigureChildFunc
	budget         core.Budget
	extensions     []extensionEntry
	chatMiddleware *core.ChatMiddleware
	maxModelCalls  int
}

// snapshotProcessOptions validates the external composition boundary and
// returns the immutable container state a Process retains. Capability objects
// (extension implementations, middleware closures, and ConfigureChild) are
// intentionally not deep-copied; their contracts require lifetime safety.
func snapshotProcessOptions(options core.ProcessOptions) (processOptions, error) {
	if options.Blackboard != nil && nilvalue.Is(options.Blackboard) {
		return processOptions{}, errors.New("ProcessOptions.Blackboard is typed nil")
	}
	extensions, err := registerProcessExtensions(options.Extensions)
	if err != nil {
		return processOptions{}, err
	}
	chatMiddleware := cloneChatMiddleware(options.ChatMiddleware)
	if options.MaxModelCalls < 0 {
		return processOptions{}, errors.New("ProcessOptions.MaxModelCalls must not be negative")
	}
	budget := options.Budget
	if err := budget.Validate(); err != nil {
		return processOptions{}, fmt.Errorf("ProcessOptions.Budget: %w", err)
	}

	return processOptions{
		configureChild: options.ConfigureChild,
		budget:         budget,
		extensions:     extensions,
		chatMiddleware: chatMiddleware,
		maxModelCalls:  options.MaxModelCalls,
	}, nil
}

func cloneChatMiddleware(middleware *core.ChatMiddleware) *core.ChatMiddleware {
	if middleware == nil {
		return nil
	}
	snapshot := *middleware
	snapshot.CallMiddlewares = slices.Clone(middleware.CallMiddlewares)
	snapshot.StreamMiddlewares = slices.Clone(middleware.StreamMiddlewares)
	return &snapshot
}

// prepareProcessDependencies closes engine composition, validates that an
// optional host-built process scope descends from it, and closes every scope on
// that path before execution begins.
//
// The runtime needs the scope to resolve engine registrations, which requires
// ancestry and nothing more: how many host-defined intermediate scopes exist
// before this process is the host's own composition to decide. Every layer up
// to the engine is frozen, because a
// scope left open could change what an already-running process resolves.
func (e *Engine) prepareProcessDependencies(configured *core.Dependencies) (*core.Dependencies, error) {
	e.dependencies.Freeze()
	if configured == nil {
		configured = e.dependencies.Child()
		configured.Freeze()
		return configured, nil
	}
	if !dependencyScopeDescendsFrom(configured, e.dependencies) {
		return nil, errors.New("process dependencies must descend from engine dependencies")
	}
	for scope := configured; scope != e.dependencies; scope = scope.Parent() {
		scope.Freeze()
	}
	return configured, nil
}

func dependencyScopeDescendsFrom(scope, ancestor *core.Dependencies) bool {
	for current := scope; current != nil; current = current.Parent() {
		if current == ancestor {
			return true
		}
	}
	return false
}
