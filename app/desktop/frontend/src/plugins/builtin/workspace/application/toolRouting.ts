import type { ToolCall } from "@/protocol/run/viewState";
import { focusWorkspaceFile, openWorkspaceViewBeside, selectWorkspaceTool } from "./navigation";
import { decideWorkspaceToolRoute } from "./toolRouteDecision";

export function openWorkspaceViewForTool(tool: ToolCall): void {
  const route = decideWorkspaceToolRoute(tool);
  if (!route) return;

  selectWorkspaceTool(tool.id);
  if (route.activeFile) focusWorkspaceFile(route.activeFile);
  openWorkspaceViewBeside(route.view);
}
