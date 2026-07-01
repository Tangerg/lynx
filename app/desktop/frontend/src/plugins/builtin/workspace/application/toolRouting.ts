import type { ToolCall } from "@/protocol/run/viewState";
import { useSessionStore } from "@/state/sessionStore";
import { decideWorkspaceToolRoute } from "./toolRouteDecision";

export function openWorkspaceViewForTool(tool: ToolCall): void {
  const route = decideWorkspaceToolRoute(tool);
  if (!route) return;

  const ui = useSessionStore.getState();
  ui.setSelectedToolId(tool.id);
  if (route.activeFile) ui.setActiveFile(route.activeFile);
  ui.openMainViewBeside(route.view);
}
