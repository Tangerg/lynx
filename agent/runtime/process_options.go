package runtime

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// processOptions is the runtime-owned subset of [core.ProcessOptions]. The
// public struct is a construction DTO; retaining it directly would let caller
// slice, pointer, and map mutations rewrite a live Process after construction.
// Blackboard and Dependencies are prepared into dedicated Process fields and
// therefore do not appear here.
type processOptions struct {
	childOptions core.ChildOptionsFunc
	budget       core.Budget
	session      *core.Session
	extensions   []core.Extension
	guardrails   *core.ChatGuardrails
}

// snapshotProcessOptions validates the external composition boundary and
// returns the immutable container state a Process retains. Capability objects
// (extension implementations, middleware closures, and ChildOptions) are
// intentionally not deep-copied; their contracts require lifetime safety.
func snapshotProcessOptions(options core.ProcessOptions) (processOptions, error) {
	if options.Blackboard != nil && valueIsNil(options.Blackboard) {
		return processOptions{}, errors.New("ProcessOptions.Blackboard is typed nil")
	}
	if err := validateProcessExtensions(options.Extensions); err != nil {
		return processOptions{}, err
	}
	guardrails, err := snapshotChatGuardrails("ProcessOptions.Guardrails", options.Guardrails)
	if err != nil {
		return processOptions{}, err
	}
	budget := options.Budget
	if budget == (core.Budget{}) {
		budget = core.DefaultBudget()
	}

	var session *core.Session
	if options.Session != nil {
		sessionSnapshot := *options.Session
		// ProcessContext exposes only SessionInfo. Opaque host metadata belongs
		// to SessionStore and must not become mutable execution-aggregate state.
		sessionSnapshot.Metadata = nil
		session = &sessionSnapshot
	}

	return processOptions{
		childOptions: options.ChildOptions,
		budget:       budget,
		session:      session,
		extensions:   slices.Clone(options.Extensions),
		guardrails:   guardrails,
	}, nil
}

// snapshotChatGuardrails validates and detaches the mutable config containers.
// label identifies the public boundary in the returned error.
func snapshotChatGuardrails(label string, guardrails *core.ChatGuardrails) (*core.ChatGuardrails, error) {
	if guardrails == nil {
		return nil, nil
	}
	if guardrails.MaxToolRounds < 0 {
		return nil, fmt.Errorf("%s.MaxToolRounds must not be negative", label)
	}
	snapshot := *guardrails
	snapshot.CallMiddlewares = slices.Clone(guardrails.CallMiddlewares)
	snapshot.StreamMiddlewares = slices.Clone(guardrails.StreamMiddlewares)
	return &snapshot, nil
}

// prepareProcessDependencies closes engine composition, validates an optional
// host-built process scope, and closes that scope before execution begins.
func (e *Engine) prepareProcessDependencies(configured *core.Dependencies) (*core.Dependencies, error) {
	e.dependencies.Freeze()
	if configured == nil {
		configured = e.dependencies.Child()
	} else if configured.Parent() != e.dependencies {
		return nil, errors.New("process dependencies must be an immediate child of engine dependencies")
	}
	configured.Freeze()
	return configured, nil
}
