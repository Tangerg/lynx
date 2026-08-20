import type { WorkspaceToolActivity } from "./toolActivity";

type WorkspaceToolViewId = string;

export interface WorkspaceToolRoute {
  view: WorkspaceToolViewId;
  fileFocus?: string;
}

export function hasWorkspaceToolView(tool: WorkspaceToolActivity): boolean {
  return tool.category === "command" || tool.category === "fileEdit" || tool.category === "read";
}

export function decideWorkspaceToolRoute(tool: WorkspaceToolActivity): WorkspaceToolRoute | null {
  if (tool.category === "command") {
    return {
      view: "terminal",
    };
  }

  if (tool.category === "fileEdit" || tool.category === "read") {
    return {
      view: "diff",
      // The Agent projection that selected the label also owns whether it is a
      // path. Do not infer path identity from translated counts, slashes, or a
      // tool name: a multi-file apply_patch deliberately labels itself as text.
      fileFocus: tool.labelKind === "path" ? tool.label : "",
    };
  }

  return null;
}
