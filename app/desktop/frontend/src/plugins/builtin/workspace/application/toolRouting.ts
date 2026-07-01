import type { ToolCall } from "@/plugins/sdk/types/agentView";
import { focusWorkspaceFile, openWorkspaceViewBeside, selectWorkspaceTool } from "./navigation";
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
  if (route.activeFile) focusWorkspaceFile(route.activeFile);
  openWorkspaceViewBeside(route.view);
}
