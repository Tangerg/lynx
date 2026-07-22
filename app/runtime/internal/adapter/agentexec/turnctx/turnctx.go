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
// `task` sub-agents: [core.Blackboard.Clone] copies protected entries onto the
// child and the action's ClearWorkingState policy preserves them, so a plain Set would
// be lost when the sub-agent's action clears its inherited blackboard.
const CwdBindingKey = "lyra:cwd"

// SessionBindingKey is the blackboard key the chat action binds (protected) with
// the turn's session id, so the read/edit guards can key file-read state per
// session — read in the same seam as the working directory. Protected so it rides
// to `task` sub-agents and survives the snapshot/resume round trip.
const SessionBindingKey = "lyra:session"

// IsolatedBindingKey is the blackboard key the chat action binds (protected)
// when the turn runs in an isolated session: its shell commands must be
// OS-jailed (network denied, workspace-write only) regardless of the global
// sandbox opt-in, and its working directory ([CwdBindingKey]) is the sandbox
// copy. Protected so a `task` sub-agent inherits the isolation.
const IsolatedBindingKey = "lyra:isolated"

// GoalLeaseBindingKey is the blackboard key the chat action binds (protected)
// on a Goal-mode autonomous run: the goal incarnation lease
// the loop launched this run under. update_goal reads it and compare-and-swaps
// on it, so a straggler run from a superseded goal cannot signal a newer goal.
// Absent on ordinary runs — then update_goal signals whatever goal is currently
// active (a user-initiated turn legitimately targets the live goal).
const GoalLeaseBindingKey = "lyra:goal-lease"

// TurnCwd reads the working directory the running process seeded on its
// blackboard ([CwdBindingKey]), falling back to fallback when the turn carried
// none (a sessionless smoke run, or a restored continuation whose snapshot
// predates cwd seeding). This is THE per-session-cwd seam: the tool resolver,
// the skill tool, and the system-prompt composition all read the same key, so
// everything cwd-dependent follows the session together.
func TurnCwd(ctx context.Context, fallback string) string {
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return fallback
	}
	if value, ok := process.Blackboard().Load(CwdBindingKey); ok {
		if cwd, ok := value.(string); ok && cwd != "" {
			return cwd
		}
	}
	return fallback
}

// TurnIsolated reports whether the running turn is in an isolated session
// ([IsolatedBindingKey]) — the shell tool passes it to [exec.Shells.Launch] so
// the command is OS-jailed. False for a normal (non-isolated) turn.
func TurnIsolated(ctx context.Context) bool {
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return false
	}
	if value, ok := process.Blackboard().Load(IsolatedBindingKey); ok {
		if isolated, ok := value.(bool); ok {
			return isolated
		}
	}
	return false
}

// TurnGoalLease reports the goal incarnation this run was launched under
// ([GoalLeaseBindingKey]) and whether it was set. update_goal uses the lease to
// reject a superseded run; ("", false) means the run carries no goal stamp
// (a user turn), so update_goal targets the currently-active goal instead.
func TurnGoalLease(ctx context.Context) (string, bool) {
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return "", false
	}
	if value, ok := process.Blackboard().Load(GoalLeaseBindingKey); ok {
		if leaseID, ok := value.(string); ok && leaseID != "" {
			return leaseID, true
		}
	}
	return "", false
}

// TurnSession reads the session id the chat action seeded on the blackboard
// ([SessionBindingKey]), empty when the turn carried none (a sessionless smoke
// run). The read/edit guards key per-session file-read state off it.
func TurnSession(ctx context.Context) string {
	process := core.ProcessViewFrom(ctx)
	if process == nil {
		return ""
	}
	if value, ok := process.Blackboard().Load(SessionBindingKey); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return ""
}
