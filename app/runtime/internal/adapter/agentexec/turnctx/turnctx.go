// Package turnctx is the per-turn context seam: the blackboard keys the chat
// action binds on a running process (working directory, session id) and the
// readers that pull them back at tool-resolution / prompt-composition time.
// It's a leaf — it depends only on the agent SDK's process/blackboard — so
// every reader (the tool resolver, the per-tool packages, the engine's
// system-prompt composition) imports it inward without coupling to each other.
package turnctx

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
)

// CwdBindingKey is the blackboard key the chat action binds (protected) with the
// turn's working directory. The resolver reads it so the filesystem + shell tools
// operate in the session's project directory. Binding it protected carries it to
// `task` sub-agents: [core.Blackboard.Spawn] copies protected entries onto the
// child and the typed-action ClearBlackboard preserves them, so a plain Set would
// be lost when the sub-agent's action clears its inherited blackboard.
const CwdBindingKey = "lyra:cwd"

// SessionBindingKey is the blackboard key the chat action binds (protected) with
// the turn's session id, so the read/edit guards can key file-read state per
// session — read in the same seam as the working directory. Protected so it rides
// to `task` sub-agents and survives the snapshot/resume round trip.
const SessionBindingKey = "lyra:session"

// TurnCwd reads the working directory the running process seeded on its
// blackboard ([CwdBindingKey]), falling back to fallback when the turn carried
// none (a sessionless smoke run, or a restored continuation whose snapshot
// predates cwd seeding). This is THE per-session-cwd seam: the tool resolver,
// the skill tool, and the system-prompt composition all read the same key, so
// everything cwd-dependent follows the session together.
func TurnCwd(ctx context.Context, fallback string) string {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return fallback
	}
	if v, ok := p.Blackboard().Get(CwdBindingKey); ok {
		if cwd, ok := v.(string); ok && cwd != "" {
			return cwd
		}
	}
	return fallback
}

// TurnSession reads the session id the chat action seeded on the blackboard
// ([SessionBindingKey]), empty when the turn carried none (a sessionless smoke
// run). The read/edit guards key per-session file-read state off it.
func TurnSession(ctx context.Context) string {
	p := core.ProcessFrom(ctx)
	if p == nil {
		return ""
	}
	if v, ok := p.Blackboard().Get(SessionBindingKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}
