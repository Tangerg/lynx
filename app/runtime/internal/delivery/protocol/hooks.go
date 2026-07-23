package protocol

import "context"

// Hooks is the hooks.* method group.
type Hooks interface {
	ListHooks(ctx context.Context, in ListHooksRequest) (*HooksListResult, error)
	SetHookTrust(ctx context.Context, in SetHookTrustRequest) error
}

// Lifecycle-hooks management wire types (hooks.*, API.md §7.5). The
// runtime runs user-authored hooks at fixed turn lifecycle points; these
// methods let a client review what's configured for a cwd and toggle whether a
// project's hooks are trusted to run (a cloned repo's hooks stay inert until
// trusted).

// ListHooksRequest — hooks.list body. Cwd scopes project discovery
// (empty = the runtime server cwd); global ~/.lyra hooks are always included.
type ListHooksRequest struct {
	Cwd string `json:"cwd,omitempty"`
}

type HookEvent string

const (
	HookEventPreToolUse       HookEvent = "PreToolUse"
	HookEventPostToolUse      HookEvent = "PostToolUse"
	HookEventUserPromptSubmit HookEvent = "UserPromptSubmit"
	HookEventSessionStart     HookEvent = "SessionStart"
	HookEventSubagentStart    HookEvent = "SubagentStart"
	HookEventSubagentStop     HookEvent = "SubagentStop"
	HookEventPreCompact       HookEvent = "PreCompact"
	HookEventStop             HookEvent = "Stop"
	HookEventNotification     HookEvent = "Notification"
)

// HookInfo is one discovered hook (global or project), for review. Command is
// the shell command a hook runs (shown so the user can audit a project's hooks
// before trusting); inject is the declarative no-exec context alternative.
type HookInfo struct {
	Event     HookEvent `json:"event"`
	Matcher   string    `json:"matcher,omitempty"`
	Command   string    `json:"command,omitempty"`
	Inject    string    `json:"inject,omitempty"`
	TimeoutMs int       `json:"timeoutMs,omitempty"`
	Scope     string    `json:"scope"`  // "global" | "project"
	Source    string    `json:"source"` // absolute path of the hooks.json it came from
	// Active reports whether this hook currently runs: global hooks always do;
	// project hooks only when the project is trusted.
	Active bool `json:"active"`
}

// HooksListResult — hooks.list result. ProjectRoot is the trust key
// (the nearest .git ancestor of the cwd); ProjectTrusted reports whether its
// project-scope hooks are enabled.
type HooksListResult struct {
	ProjectRoot    string     `json:"projectRoot,omitempty"`
	ProjectTrusted bool       `json:"projectTrusted"`
	Hooks          []HookInfo `json:"hooks"`
}

// SetHookTrustRequest — hooks.setTrust body: trust (or revoke) a
// project's hooks. ProjectRoot is the value HooksListResult reported.
type SetHookTrustRequest struct {
	ProjectRoot string `json:"projectRoot"`
	Trusted     bool   `json:"trusted"`
}
