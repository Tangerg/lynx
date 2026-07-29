export type WorkspaceInvalidationTarget =
  | "all"
  | "diff"
  | "filesChanged"
  | "mcpConfigs"
  | "mcpServers"
  | "mcpTools"
  | "schedules"
  | "sessions"
  | "skills"
  | "managedSkills"
  | "skillDrafts";

export interface WorkspaceEventLike {
  type: string;
  sequence: number;
}

// Every runtime signal is an invalidation: it says a resource moved, and the reads it
// feeds are what has to be asked again. The signal carries no values, so this is the
// whole mapping — there is nothing to merge, and nothing that can be stale in a way
// the next read would not fix.
//
// A topic maps here only if this client HAS a read for it. runs / interrupts / goals /
// state are folded from the run stream today, so a signal about them would ask for a
// refetch of nothing; the subscription does not request those topics rather than
// requesting them and dropping them (§7.2 — no wildcard, ask for what you fold).
export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff"];
    case "skills.changed":
      return ["skills", "managedSkills", "skillDrafts"];
    case "mcp.changed":
      return ["mcpServers", "mcpConfigs", "mcpTools"];
    case "schedules.changed":
      // A schedule that fired starts a run in a fresh session, so the session list
      // moves with it.
      return ["schedules", "sessions"];
    case "sessions.changed":
      return ["sessions"];
    case "resync":
      // The signal names the topics that went stale, but a client that fell behind on
      // one may have fallen behind on more: this stream is lossy by design, so the
      // honest response to "you missed something" is to read everything again.
      return ["all"];
    default:
      return [];
  }
}
