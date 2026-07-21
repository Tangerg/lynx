export type WorkspaceInvalidationTarget =
  | "all"
  | "diff"
  | "filesChanged"
  | "mcpConfigs"
  | "mcpServers"
  | "mcpTools"
  | "sessions"
  | "skills"
  | "managedSkills"
  | "skillDrafts";

export interface WorkspaceEventLike {
  type: string;
  sequence: number;
}

export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff"];
    case "skills.changed":
      return ["skills", "managedSkills", "skillDrafts"];
    case "mcp.serverChanged":
      return ["mcpServers", "mcpConfigs", "mcpTools"];
    case "schedules.fired":
      return ["sessions"];
    case "resync":
      return ["all"];
    default:
      return [];
  }
}
