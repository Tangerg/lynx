import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { focusWorkspaceFile, openWorkspaceViewInDock, selectWorkspaceTool } from "./navigation";
import { decideWorkspaceToolRoute, hasWorkspaceToolView } from "./toolRouteDecision";
import { workspaceToolActivityFromAgentTool } from "./toolActivity";

export function hasWorkspaceViewForTool(tool: ToolCall): boolean {
  return hasWorkspaceToolView(workspaceToolActivityFromAgentTool(tool));
}

export function openWorkspaceViewForTool(tool: ToolCall): void {
  const activity = workspaceToolActivityFromAgentTool(tool);
  const route = decideWorkspaceToolRoute(activity);
  if (!route) return;

  selectWorkspaceTool(activity.id);
  if (route.fileFocus !== undefined) focusWorkspaceFile(route.fileFocus);
  openWorkspaceViewInDock(route.view);
}
