// Pure dispatcher: when a tool card is clicked, pick the right workspace
// view, set context, and promote it to the main-area tab strip.
//
// Lives outside the render tree so `ToolCard` can invoke it directly
// without anyone passing a callback down through the chat panel.

import { useAgentStore } from "./agentStore";
import { useUIStore } from "./uiStore";

type ViewRouting = { id: string; title: string; icon: string };

// Choose which workspace view to surface for a given tool fn name. Each
// branch sets up the side effects the view will rely on (e.g. active-file
// path for the diff view).
function routeForTool(fn: string, args: string): ViewRouting {
  if (fn === "bash") {
    return { id: "terminal", title: "Terminal", icon: "terminal" };
  }
  if (fn === "edit_file" || fn === "write_file" || fn === "read_file") {
    // Pull the path off the front of the args ("src/foo.ts …") so the
    // diff view knows which file to render.
    const m = args.match(/^([^\s(]+)/);
    if (m) useUIStore.getState().setActiveFile(m[1]);
    return { id: "diff", title: "Diff", icon: "diff" };
  }
  return { id: "diff", title: "Diff", icon: "diff" };
}

/**
 * Surface the workspace view that corresponds to the clicked tool.
 * No-op if the tool id isn't in the current agent state.
 */
export function openViewForTool(toolId: string): void {
  const tool = useAgentStore.getState().toolCalls[toolId];
  if (!tool) return;

  const view = routeForTool(tool.fn, String(tool.args));
  const ui = useUIStore.getState();
  ui.setSelectedToolId(toolId);
  ui.openMainView(view);
}
