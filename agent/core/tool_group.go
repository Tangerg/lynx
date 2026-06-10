package core

import (
	"context"
	"slices"
	"sync"

	"github.com/Masterminds/semver/v3"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ToolGroupPermission is the security/sensitivity flag on a ToolGroup —
// helpful so user-facing UIs can surface "this agent will need internet
// access" before kicking off a long-running task, and so platform
// validators can enforce sandbox policy at deploy time.
type ToolGroupPermission int

const (
	// ToolGroupHostAccess covers tools that touch the host process's
	// own resources: filesystem, environment, child processes, local
	// network sockets. Sandboxed deployments deny this by default.
	ToolGroupHostAccess ToolGroupPermission = iota

	// ToolGroupInternetAccess covers tools that reach off-host
	// network endpoints (web search, HTTP fetch, third-party APIs).
	// Air-gapped deployments deny this; cost-sensitive deployments
	// may track it for outbound-traffic budgeting.
	ToolGroupInternetAccess
)

func (p ToolGroupPermission) String() string {
	switch p {
	case ToolGroupHostAccess:
		return "host_access"
	case ToolGroupInternetAccess:
		return "internet_access"
	default:
		return "unknown"
	}
}

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

// ToolGroupMetadata is what a resolver returns from a registry
// lookup. Carries the role + the (optional) versioned coordinates so
// observability surfaces can display "which provider's tool group
// satisfied this role", plus the permissions the group exercises so
// the platform can refuse high-privilege groups in sandboxed deployments.
type ToolGroupMetadata interface {
	Role() string
	AssetCoordinates() AssetCoordinates

	// Permissions reports the categories of access this tool group
	// requires at runtime. An empty result means the group claims no
	// special access; the platform must still consult its own policy
	// before granting use. Callers use the returned slice as a
	// read-only snapshot — implementations should return a fresh slice
	// per call when their underlying storage is mutable.
	Permissions() []ToolGroupPermission
}

// ToolGroupRequirement is what an agent declares ("I need a
// search-shaped tool group") without binding to a specific provider.
// The planner consults the resolver to translate role → concrete
// tool list at execution time.
//
// Permissions lists the privileges the action is willing to grant the
// resolved group. The runtime rejects a group whose Metadata().Permissions()
// is not a subset of this set, so a sandboxed agent can't accidentally
// pick up an internet-reaching resolver implementation. An empty
// Permissions slice means "no special privileges" — high-privilege
// groups are rejected unless the requirement opts in.
type ToolGroupRequirement struct {
	Role        string
	Permissions []ToolGroupPermission
}

// PermissionsSatisfy returns true when every permission in `granted`
// also appears in `required`. Empty `granted` is always satisfied.
// Order does not matter.
func PermissionsSatisfy(required, granted []ToolGroupPermission) bool {
	for _, g := range granted {
		if !slices.Contains(required, g) {
			return false
		}
	}
	return true
}

// TerminationScope is the structured-termination enum: AGENT stops
// the whole process; ACTION skips the current action and re-plans.
type TerminationScope int

const (
	// TerminationScopeAgent stops the entire process when the
	// trigger fires.
	TerminationScopeAgent TerminationScope = iota

	// TerminationScopeAction skips the current action and re-plans.
	TerminationScopeAction
)

func (t TerminationScope) String() string {
	switch t {
	case TerminationScopeAgent:
		return "agent"
	case TerminationScopeAction:
		return "action"
	default:
		return "unknown"
	}
}

// TerminationSignal is the per-instance termination payload. Agents
// expose TerminateAgent/TerminateAction methods that enqueue one of these
// for the runtime to pick up at the next tick boundary.
type TerminationSignal struct {
	Scope  TerminationScope
	Reason string
}

// AgentTool is an alias of [chat.Tool] — the agent runtime and the
// chat package share one tool model. Tools flow through the same
// [ToolGroup] / [ToolGroupResolver] / [ToolDecorator] machinery.
//
// Construct concrete tools via [chat.NewTool]; the runtime never builds
// AgentTool literals itself.
type AgentTool = chat.Tool

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

// ToolGroupResolver maps a requirement to a concrete group. Registered
// as a platform extension; the runtime walks every registered resolver
// in registration order and the first one returning a non-nil group
// wins. Resolvers double as [Extension] so the dispatch site can
// attribute hits / errors by Name.
type ToolGroupResolver interface {
	Extension

	Resolve(ctx context.Context, req ToolGroupRequirement) (ToolGroup, error)
}

// StaticToolGroupResolver is the in-process default — a map of role →
// ToolGroup. It is sufficient for unit tests and small deployments; larger
// fleets supply a custom resolver that talks to a registry.
type StaticToolGroupResolver struct {
	name   string
	mu     sync.RWMutex
	groups map[string]ToolGroup
}

// NewStaticToolGroupResolver returns an empty resolver with the supplied
// extension Name (used by the runtime for dedup / logging — defaults to
// "static-tool-group-resolver" when blank). Use Register to populate.
func NewStaticToolGroupResolver(name string) *StaticToolGroupResolver {
	if name == "" {
		name = "static-tool-group-resolver"
	}
	return &StaticToolGroupResolver{name: name, groups: map[string]ToolGroup{}}
}

// Name implements [Extension].
func (r *StaticToolGroupResolver) Name() string { return r.name }

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

// SimpleToolGroupMetadata is the minimal metadata struct — used by
// the static resolver and by adapters (mcp, subagent) that don't
// version their tool groups.
type SimpleToolGroupMetadata struct {
	RoleText           string
	Coordinates        AssetCoordinates
	PermissionsGranted []ToolGroupPermission
}

func (m SimpleToolGroupMetadata) Role() string                       { return m.RoleText }
func (m SimpleToolGroupMetadata) AssetCoordinates() AssetCoordinates { return m.Coordinates }

// Permissions returns the permission slice verbatim — callers are
// expected to treat the result as read-only. SimpleToolGroupMetadata
// is a value type used for static configuration, so sharing the slice
// across callers is acceptable.
func (m SimpleToolGroupMetadata) Permissions() []ToolGroupPermission {
	return m.PermissionsGranted
}
