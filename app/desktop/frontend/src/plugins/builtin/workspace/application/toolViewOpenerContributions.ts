import type { ToolViewOpenerSpec } from "@/plugins/sdk";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "./toolRouting";

export function workspaceToolViewOpener(): ToolViewOpenerSpec {
  return {
    id: "workspace-tool-view",
    order: 0,
    predicate: hasWorkspaceViewForTool,
    open: openWorkspaceViewForTool,
  };
}
