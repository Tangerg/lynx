package core

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/Masterminds/semver/v3"

	"github.com/Tangerg/lynx/tools"
)

// ToolGroupPermission is the security/sensitivity flag on a ToolGroup —
// helpful so user-facing UIs can surface "this agent will need internet
// access" before kicking off a long-running task, and so engine
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

// ToolGroupRef is the (provider, name, version) triple that uniquely
// identifies a ToolGroup release across distribution channels. Version
// is a [*semver.Version] so multi-version registries (e.g. several
// MCP-server builds in the same resolver) can sort and range over them
// without parsing strings.
type ToolGroupRef struct {
	Provider string
	Name     string
	Version  *semver.Version
}

// ToolGroupInfo describes a resolved tool group. Role is the abstract
// capability requested by an action; Ref optionally identifies the concrete
// provider release; Permissions declares the access required at runtime.
type ToolGroupInfo struct {
	Role        string
	Ref         ToolGroupRef
	Permissions []ToolGroupPermission
}

// ToolGroupRequirement is what an agent declares ("I need a
// search-shaped tool group") without binding to a specific provider.
// The planner consults the resolver to translate role → concrete
// tool list at execution time.
//
// AllowedPermissions lists the privileges the action is willing to grant the
// resolved group. The runtime rejects a group whose Info().Permissions
// is not a subset of this set, so a sandboxed agent can't accidentally
// pick up an internet-reaching resolver implementation. An empty
// AllowedPermissions slice means "no special privileges" — high-privilege
// groups are rejected unless the requirement opts in.
type ToolGroupRequirement struct {
	Role               string
	AllowedPermissions []ToolGroupPermission
}

// AllowsPermissions reports whether allowed contains every required
// permission. An empty required set is always allowed.
// Order does not matter.
func AllowsPermissions(allowed, required []ToolGroupPermission) bool {
	for _, permission := range required {
		if !slices.Contains(allowed, permission) {
			return false
		}
	}
	return true
}

// ToolGroup is the lazy provider — Tools(ctx) is the entry point that
// performs the (potentially expensive) MCP handshake / plugin load on first
// access. Subsequent calls return the cached slice.
type ToolGroup interface {
	Info() ToolGroupInfo
	Tools(ctx context.Context) ([]tools.Tool, error)
}

// LazyToolGroup is a ready-made ToolGroup that resolves its tool list once
// and caches the result. Most callers use this rather than building a fresh
// implementation.
type LazyToolGroup struct {
	info ToolGroupInfo
	load func(ctx context.Context) ([]tools.Tool, error)

	once    sync.Once
	tools   []tools.Tool
	loadErr error
}

// NewLazyToolGroup wraps an info and loader pair. The loader runs at most
// once per LazyToolGroup instance, on the first Tools() call.
func NewLazyToolGroup(info ToolGroupInfo, load func(ctx context.Context) ([]tools.Tool, error)) *LazyToolGroup {
	info.Permissions = slices.Clone(info.Permissions)
	return &LazyToolGroup{info: info, load: load}
}

// Info returns a defensive copy of the group description.
func (l *LazyToolGroup) Info() ToolGroupInfo {
	info := l.info
	info.Permissions = slices.Clone(info.Permissions)
	return info
}

func (l *LazyToolGroup) Tools(ctx context.Context) ([]tools.Tool, error) {
	l.once.Do(func() {
		if l.load == nil {
			return
		}
		l.tools, l.loadErr = l.load(ctx)
	})
	return slices.Clone(l.tools), l.loadErr
}

// ToolGroupResolver maps a requirement to a concrete group. Registered
// as an engine extension; the runtime walks every registered resolver
// in registration order and the first one returning a non-nil group
// wins. Resolvers double as [Extension] so the dispatch site can
// attribute hits / errors by Name.
type ToolGroupResolver interface {
	Extension

	Resolve(ctx context.Context, requirement ToolGroupRequirement) (ToolGroup, bool, error)
}

// LazyToolGroupResolver resolves one metadata-described role to a fresh
// lazy group backed by load. It is the generic adapter for remote registries,
// plug-in catalogs, MCP sessions, or any other dynamic tool source; transport
// details remain in the caller-provided loader.
type LazyToolGroupResolver struct {
	name string
	info ToolGroupInfo
	load func(context.Context) ([]tools.Tool, error)
}

// NewLazyToolGroupResolver validates and constructs a one-role resolver.
// Each successful Resolve returns an independent LazyToolGroup, whose loader
// runs once on first use.
func NewLazyToolGroupResolver(
	name string,
	info ToolGroupInfo,
	load func(context.Context) ([]tools.Tool, error),
) (*LazyToolGroupResolver, error) {
	if name == "" {
		return nil, errors.New("core.NewLazyToolGroupResolver: name must not be empty")
	}
	if info.Role == "" {
		return nil, errors.New("core.NewLazyToolGroupResolver: role must not be empty")
	}
	if load == nil {
		return nil, errors.New("core.NewLazyToolGroupResolver: loader must not be nil")
	}
	info.Permissions = slices.Clone(info.Permissions)
	return &LazyToolGroupResolver{name: name, info: info, load: load}, nil
}

func (r *LazyToolGroupResolver) Name() string { return r.name }

func (r *LazyToolGroupResolver) Resolve(_ context.Context, requirement ToolGroupRequirement) (ToolGroup, bool, error) {
	if requirement.Role != r.info.Role {
		return nil, false, nil
	}
	return NewLazyToolGroup(r.info, r.load), true, nil
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
// extension name. Use Set to populate it. A blank name selects a stable
// default.
func NewStaticToolGroupResolver(name string) *StaticToolGroupResolver {
	if name == "" {
		name = "static-tool-group-resolver"
	}
	return &StaticToolGroupResolver{name: name, groups: map[string]ToolGroup{}}
}

// Name implements [Extension].
func (r *StaticToolGroupResolver) Name() string { return r.name }

// Set binds group to role, replacing any previous binding.
func (r *StaticToolGroupResolver) Set(role string, group ToolGroup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[role] = group
}

// Resolve returns (group, true, nil) for a known role.
// (nil group, false, nil) reports a miss so the caller can continue to
// the next resolver.
func (r *StaticToolGroupResolver) Resolve(_ context.Context, requirement ToolGroupRequirement) (ToolGroup, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	group, ok := r.groups[requirement.Role]
	return group, ok, nil
}
