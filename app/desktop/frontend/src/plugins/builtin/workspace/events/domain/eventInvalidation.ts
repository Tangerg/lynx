export type WorkspaceInvalidationTarget =
  | "all"
  | "agentSessionProjection"
  | "diff"
  | "filesChanged"
  | "goal"
  | "mcpServers"
  | "mcpTools"
  | "pendingWork"
  | "schedules"
  | "sessionUsage"
  | "sessions"
  | "skills"
  | "managedSkills"
  | "skillProposals";

// The change signals this app can fold, as a closed set. It is spelled here rather
// than imported from the wire so this layer stays protocol-free — and the assignment
// at the subscribe adapter is then the drift gate: a signal the runtime adds shows up
// as a type error at the boundary instead of silently reaching a default branch.
export type WorkspaceEventType =
  | "files.changed"
  | "skills.changed"
  | "mcp.changed"
  | "schedules.changed"
  | "sessions.changed"
  | "runs.changed"
  | "state.changed"
  | "goals.changed"
  | "interrupts.changed"
  | "resync";

export interface WorkspaceEventLike {
  type: WorkspaceEventType;
  sequence: number;
  sessionIds?: string[];
}

// Every runtime signal is an invalidation: it says a resource moved, and the reads it
// feeds are what has to be asked again. The signal carries no values, so this is the
// whole mapping — there is nothing to merge, and nothing that can be stale in a way
// the next read would not fix.
//
// The switch is exhaustive by construction (the default branch only type-checks while
// every member is handled): a topic with no read is a signal this client asked for and
// then dropped, which is indistinguishable from a bug.
export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff"];
    case "skills.changed":
      return ["skills", "managedSkills", "skillProposals"];
    case "mcp.changed":
      return ["mcpServers", "mcpTools"];
    case "schedules.changed":
      // A schedule that fired starts a run in a fresh session, so the session list
      // moves with it.
      return ["schedules", "sessions"];
    case "sessions.changed":
      return ["sessions"];
    case "runs.changed":
      // A run's position is what a session row reports as its status, and a run that
      // ended changed what the session has spent.
      // A run that ended cannot still be waiting on anyone.
      return ["sessions", "sessionUsage", "agentSessionProjection", "pendingWork"];
    case "interrupts.changed":
      // A session waiting on a person reads differently in the list than one working,
      // and the queue of what is waiting is this signal's whole subject.
      return ["sessions", "agentSessionProjection", "pendingWork"];
    case "goals.changed":
      // This is why the goal banner no longer polls: an autonomous loop moves a goal
      // between turns, and the signal says so as it happens.
      return ["goal"];
    case "state.changed":
      return ["agentSessionProjection"];
    case "resync":
      // The signal names the topics that went stale, but a client that fell behind on
      // one may have fallen behind on more: this stream is lossy by design, so the
      // honest response to "you missed something" is to read everything again.
      return ["all"];
    default: {
      const unhandled: never = ev.type;
      return unhandled;
    }
  }
}
