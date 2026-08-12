export type WorkspaceInvalidationTarget =
  | "all"
  | "agentMemory"
  | "agentSessionProjection"
  | "agentDocs"
  | "diff"
  | "fileHead"
  | "fileList"
  | "fileRead"
  | "filesChanged"
  | "goal"
  | "grep"
  | "hooks"
  | "knowledge"
  | "mcpServers"
  | "mcpTools"
  | "pendingWork"
  | "recipes"
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
  | "knowledge.changed"
  | "hooks.changed"
  | "resync";

export type WorkspaceTopic = Exclude<WorkspaceEventType, "resync">;

export interface WorkspaceEventLike {
  type: WorkspaceEventType;
  sequence: number;
  sessionIds?: string[];
  topics?: WorkspaceTopic[];
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
      // Every read below is derived from files under the workspace. Keeping only
      // the VCS projections fresh leaves an open file, the lazy tree, completion,
      // search, and file-backed catalogs stale for their five-minute cache life.
      return [
        "filesChanged",
        "diff",
        "fileList",
        "fileRead",
        "fileHead",
        "grep",
        "recipes",
        "hooks",
        "knowledge",
        "agentDocs",
        "skills",
      ];
    case "skills.changed":
      return ["skills", "managedSkills", "skillProposals"];
    case "mcp.changed":
      return ["mcpServers", "mcpTools"];
    case "schedules.changed":
      return ["schedules"];
    case "sessions.changed":
      return ["sessions"];
    case "runs.changed":
      // Completed Runs can also commit agent-memory maintenance before this
      // signal is published. The Runtime publishes sessions.changed separately
      // for the Session list and interrupts.changed separately for pending human
      // work; invalidating those here as well duplicates every lifecycle read
      // and reintroduces races between two refetches of the same resource.
      return ["sessionUsage", "agentMemory", "agentSessionProjection"];
    case "interrupts.changed":
      return ["agentSessionProjection", "pendingWork"];
    case "goals.changed":
      // This is why the goal banner no longer polls: an autonomous loop moves a goal
      // between turns, and the signal says so as it happens.
      return ["goal"];
    case "state.changed":
      return ["agentSessionProjection"];
    case "knowledge.changed":
      return ["knowledge"];
    case "hooks.changed":
      return ["hooks"];
    case "resync": {
      // Resync is already the runtime's exact loss projection: it names every
      // topic that was folded while this subscriber's queue was full. Widening
      // that scope to every query creates false dependencies between unrelated
      // read models and can turn one watched Git change into a refetch loop.
      if (!ev.topics?.length) return ["all"];
      const targets = new Set<WorkspaceInvalidationTarget>();
      for (const topic of ev.topics) {
        for (const target of workspaceInvalidations({ type: topic, sequence: ev.sequence })) {
          targets.add(target);
        }
      }
      return [...targets];
    }
    default: {
      const unhandled: never = ev.type;
      return unhandled;
    }
  }
}
