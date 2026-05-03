package core

import (
	"context"
	"sync"

	"github.com/Masterminds/semver/v3"
)

// ToolGroupPermission is the security/sensitivity flag on a ToolGroup —
// helpful so user-facing UIs can surface "this agent will need internet
// access" before kicking off a long-running task.
type ToolGroupPermission int

const (
	ToolGroupHostAccess ToolGroupPermission = iota
	ToolGroupInternetAccess
)

// AssetCoordinates is the (provider, name, version) triple that uniquely
// identifies a ToolGroup release across distribution channels. Version
// is a [*semver.Version] so multi-version registries (e.g. several
// MCP-server builds in the same resolver) can sort and range over them
// without parsing strings.
type AssetCoordinates struct {
	Provider string
	Name     string
	Version  *semver.Version
}

// ToolGroupDescription carries human-facing prose. Separated from metadata
// because some lightweight integrations only need the description.
type ToolGroupDescription interface {
	Description() string
	Role() string
	ChildToolUsageNotes() string
}

// ToolGroupMetadata is what a resolver returns from a registry lookup.
type ToolGroupMetadata interface {
	ToolGroupDescription
	AssetCoordinates() AssetCoordinates
	Permissions() []ToolGroupPermission
}

// ToolGroupRequirement is what an agent declares ("I need a search-shaped
// tool group") without binding to a specific provider. The planner consults
// the resolver to translate role → concrete tool list at execution time.
type ToolGroupRequirement struct {
	Role              string
	RequiredToolNames []string
	TerminationScope  TerminationScope
}

// TerminationScope is embabel 0.4's structured-termination enum: AGENT means
// missing tooling kills the process, ACTION means the planner should skip
// the affected action and re-plan.
type TerminationScope int

const (
	// TerminationScopeAgent stops the entire process when the trigger
	// fires. The strongest guarantee: no further actions, no replanning.
	TerminationScopeAgent TerminationScope = iota

	// TerminationScopeAction skips the current action and re-plans. Use
	// when a missing tool / unmet precondition makes this action unable
	// to proceed but the agent as a whole can still pursue the goal via
	// a different path.
	TerminationScopeAction

	// TerminationScopeToolCall aborts the in-flight tool invocation but
	// lets the action body continue (typically retrying with a different
	// tool or a degraded mode). Mirrors embabel's TOOLCALL granularity —
	// the finest-grained scope.
	TerminationScopeToolCall
)

func (t TerminationScope) String() string {
	switch t {
	case TerminationScopeAgent:
		return "agent"
	case TerminationScopeAction:
		return "action"
	case TerminationScopeToolCall:
		return "tool_call"
	default:
		return "unknown"
	}
}

// TerminationScopeSignal is the per-instance termination payload. Agents
// expose TerminateAgent/TerminateAction methods that enqueue one of these
// for the runtime to pick up at the next tick boundary.
type TerminationScopeSignal struct {
	Scope  TerminationScope
	Reason string
}

// AgentTool is the framework-internal alias for a tool the agent can invoke.
// We don't directly import lynx/core/model/chat here so the agent module
// remains buildable standalone; concrete adapters convert chat.Tool ↔
// AgentTool. The fields mirror chat.ToolDefinition exactly.
type AgentTool struct {
	Name        string
	Description string
	InputSchema string

	// Call is the optional invocation hook. nil signals an "external" tool —
	// the runtime collects the call and lets the host application perform it.
	Call func(ctx context.Context, arguments string) (string, error)

	// ReturnDirect mirrors chat.ToolMetadata.ReturnDirect — when true, the
	// LLM should not be re-prompted with the result.
	ReturnDirect bool
}

// ToolGroup is the lazy provider — Tools(ctx) is the entry point that
// performs the (potentially expensive) MCP handshake / plugin load on first
// access. Subsequent calls return the cached slice.
type ToolGroup interface {
	Metadata() ToolGroupMetadata
	Tools(ctx context.Context) ([]AgentTool, error)
}

// LazyToolGroup is a ready-made ToolGroup that resolves its tool list once
// and caches the result. Most callers use this rather than building a fresh
// implementation.
type LazyToolGroup struct {
	meta   ToolGroupMetadata
	loadFn func(ctx context.Context) ([]AgentTool, error)

	once    sync.Once
	tools   []AgentTool
	loadErr error
}

// NewLazyToolGroup wraps a metadata+loader pair. The loader runs at most
// once per LazyToolGroup instance, on the first Tools() call.
func NewLazyToolGroup(meta ToolGroupMetadata, loadFn func(ctx context.Context) ([]AgentTool, error)) *LazyToolGroup {
	return &LazyToolGroup{meta: meta, loadFn: loadFn}
}

func (l *LazyToolGroup) Metadata() ToolGroupMetadata { return l.meta }

func (l *LazyToolGroup) Tools(ctx context.Context) ([]AgentTool, error) {
	l.once.Do(func() {
		if l.loadFn == nil {
			return
		}
		l.tools, l.loadErr = l.loadFn(ctx)
	})
	return l.tools, l.loadErr
}

// ToolGroupResolver maps a requirement to a concrete group. The platform
// holds one; tests inject a stub.
type ToolGroupResolver interface {
	Resolve(ctx context.Context, req ToolGroupRequirement) (ToolGroup, error)
}

// StaticToolGroupResolver is the in-process default — a map of role →
// ToolGroup. It is sufficient for unit tests and small deployments; larger
// fleets supply a custom resolver that talks to a registry.
type StaticToolGroupResolver struct {
	mu     sync.RWMutex
	groups map[string]ToolGroup
}

// NewStaticToolGroupResolver returns an empty resolver. Use Register to
// populate it.
func NewStaticToolGroupResolver() *StaticToolGroupResolver {
	return &StaticToolGroupResolver{groups: map[string]ToolGroup{}}
}

// Register adds (or replaces) the tool group bound to a role.
func (r *StaticToolGroupResolver) Register(role string, group ToolGroup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[role] = group
}

// Resolve returns nil group, nil error when the role is unknown — callers
// distinguish "missing" from "errored" by checking the returned group.
func (r *StaticToolGroupResolver) Resolve(_ context.Context, req ToolGroupRequirement) (ToolGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.groups[req.Role], nil
}

// SimpleToolGroupMetadata is the minimal metadata struct used by the static
// resolver and by tests when they don't need richer descriptions.
type SimpleToolGroupMetadata struct {
	DescriptionText    string
	RoleText           string
	ChildToolUsageText string
	Coordinates        AssetCoordinates
	PermissionList     []ToolGroupPermission
}

func (m SimpleToolGroupMetadata) Description() string                { return m.DescriptionText }
func (m SimpleToolGroupMetadata) Role() string                       { return m.RoleText }
func (m SimpleToolGroupMetadata) ChildToolUsageNotes() string        { return m.ChildToolUsageText }
func (m SimpleToolGroupMetadata) AssetCoordinates() AssetCoordinates { return m.Coordinates }
func (m SimpleToolGroupMetadata) Permissions() []ToolGroupPermission { return m.PermissionList }
