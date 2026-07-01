export type WorkspaceInvalidationTarget =
  | "all"
  | "diff"
  | "filesChanged"
  | "mcpConfigs"
  | "mcpServers"
  | "mcpTools"
  | "sessions"
  | "skills";

export interface WorkspaceEventLike {
  type: string;
}

export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff"];
    case "skills.changed":
      return ["skills"];
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
