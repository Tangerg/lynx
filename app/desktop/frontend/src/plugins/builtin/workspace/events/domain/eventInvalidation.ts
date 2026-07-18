export type WorkspaceInvalidationTarget =
  | "all"
  | "diff"
  | "filesChanged"
  | "mcpConfigs"
  | "mcpServers"
  | "mcpTools"
  | "sessions"
  | "skills"
  | "managedSkills";

export interface WorkspaceEventLike {
  type: string;
}

export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff"];
    case "skills.changed":
      return ["skills", "managedSkills"];
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
