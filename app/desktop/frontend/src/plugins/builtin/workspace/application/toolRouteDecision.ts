import type { ToolCall } from "@/protocol/run/viewState";
import { toolCategory } from "@/protocol/run/viewState";

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

export function hasWorkspaceToolView(tool: ToolCall): boolean {
  const category = toolCategory(tool.name);
  return category === "command" || category === "fileEdit" || category === "read";
}

export function decideWorkspaceToolRoute(tool: ToolCall): WorkspaceToolRoute | null {
  const category = toolCategory(tool.name);

  if (category === "command") {
    return {
      view: { id: "terminal", title: "workspace.view.title.terminal", icon: "terminal" },
    };
  }

  if (category === "fileEdit" || category === "read") {
    return {
      view: { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      activeFile: tool.fn && !MULTI_FILE_LABEL.test(tool.fn) ? tool.fn : undefined,
    };
  }

  return null;
}
