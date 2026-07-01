import type { WorkspaceToolActivity } from "./toolActivity";

interface WorkspaceToolView {
  id: string;
  title: string;
  icon: string;
}

export interface WorkspaceToolRoute {
  view: WorkspaceToolView;
  activeFile?: string;
}

const MULTI_FILE_LABEL = /^\d+ files$/;

export function hasWorkspaceToolView(tool: WorkspaceToolActivity): boolean {
  return tool.category === "command" || tool.category === "fileEdit" || tool.category === "read";
}

export function decideWorkspaceToolRoute(tool: WorkspaceToolActivity): WorkspaceToolRoute | null {
  if (tool.category === "command") {
    return {
      view: { id: "terminal", title: "workspace.view.title.terminal", icon: "terminal" },
    };
  }

  if (tool.category === "fileEdit" || tool.category === "read") {
    return {
      view: { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      activeFile: tool.label && !MULTI_FILE_LABEL.test(tool.label) ? tool.label : undefined,
    };
  }

  return null;
}
