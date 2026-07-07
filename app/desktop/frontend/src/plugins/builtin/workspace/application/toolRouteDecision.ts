import type { WorkspaceToolActivity } from "./toolActivity";

type WorkspaceToolViewId = string;

export interface WorkspaceToolRoute {
  view: WorkspaceToolViewId;
  activeFile?: string;
}

const MULTI_FILE_LABEL = /^\d+ files$/;

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
      activeFile: tool.label && !MULTI_FILE_LABEL.test(tool.label) ? tool.label : undefined,
    };
  }

  return null;
}
