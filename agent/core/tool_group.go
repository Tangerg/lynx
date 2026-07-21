package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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

func (p ToolGroupPermission) valid() bool {
	return p >= ToolGroupHostAccess && p <= ToolGroupInternetAccess
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

// Validate checks the universal contract every resolved group must satisfy.
func (i ToolGroupInfo) Validate() error {
	if strings.TrimSpace(i.Role) == "" {
		return errors.New("tool group info: role is empty")
	}
	if strings.TrimSpace(i.Role) != i.Role {
		return errors.New("tool group info: role has surrounding whitespace")
	}
	if err := validateToolGroupPermissions(i.Permissions); err != nil {
		return fmt.Errorf("tool group info: %w", err)
	}
	return nil
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

// Validate checks the declaration before an Agent is deployed.
func (r ToolGroupRequirement) Validate() error {
	if strings.TrimSpace(r.Role) == "" {
		return errors.New("tool group requirement: role is empty")
	}
	if strings.TrimSpace(r.Role) != r.Role {
		return errors.New("tool group requirement: role has surrounding whitespace")
	}
	if err := validateToolGroupPermissions(r.AllowedPermissions); err != nil {
		return fmt.Errorf("tool group requirement: %w", err)
	}
	return nil
}

// Allows reports whether the requirement grants every required permission.
// An empty required set is always allowed. Order does not matter.
func (r ToolGroupRequirement) Allows(required []ToolGroupPermission) bool {
	for _, permission := range required {
		if !slices.Contains(r.AllowedPermissions, permission) {
			return false
		}
	}
	return true
}

func validateToolGroupPermissions(permissions []ToolGroupPermission) error {
	for index, permission := range permissions {
		if !permission.valid() {
			return fmt.Errorf("unknown permission %d at index %d", permission, index)
		}
	}
	return nil
}

// ToolGroup describes and supplies one resolved set of tools. Implementations
// own loading, caching, retry, synchronization, and lifecycle policy. Runtime
// may call Info and Tools concurrently and does not retain or coordinate an
// implementation's mutable state.
type ToolGroup interface {
	Info() ToolGroupInfo
	Tools(ctx context.Context) ([]tools.Tool, error)
}

// ToolGroupResolver maps a requirement to a concrete group. Registered
// as an engine extension; the runtime walks every registered resolver
// in registration order and the first one returning a non-nil group
// wins. Resolvers double as [Extension] so the dispatch site can
// attribute hits / errors by Name. Panics from Resolve or from the returned
// group's Info/Tools methods become attributed resolution errors. Valid at
// engine and process scope. Implementations own source discovery, caching,
// synchronization, retry, and connection lifecycle; Runtime only validates
// and consumes the returned capability.
type ToolGroupResolver interface {
	Extension

	Resolve(ctx context.Context, requirement ToolGroupRequirement) (ToolGroup, bool, error)
}
