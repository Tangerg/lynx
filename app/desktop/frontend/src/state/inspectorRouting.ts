// Pure dispatcher: when a tool card is clicked, pick the right inspector
// view, set context, and promote it to the main-area tab strip.
//
// Used to live inline in `shell-chat` and was threaded through
// `MessageStream` → `PartCtx` → `ToolCard` as an `onOpenInspector` prop.
// Pulling it into a store-driven utility lets `ToolCard` invoke it
// directly without anyone in the render tree passing it around.

import { useAgentStore } from "./agentStore";
import { useUIStore } from "./uiStore";

type ViewRouting = { viewId: string; title: string; icon: string };

// Choose which inspector view to surface for a given tool fn name.
// Each branch sets up the side effects the view will rely on
// (active-file path for diff; selected-tool id for terminal).
function routeForTool(fn: string, args: string): ViewRouting {
  if (fn === "bash") {
    return { viewId: "terminal", title: "Terminal", icon: "terminal" };
  }
  if (fn === "edit_file" || fn === "write_file" || fn === "read_file") {
    // Pull the path off the front of the args ("src/foo.ts ...") so the
    // diff view knows which file to render.
    const m = args.match(/^([^\s(]+)/);
    if (m) useUIStore.getState().setActiveFile(m[1]);
    return { viewId: "diff", title: "Diff", icon: "diff" };
  }
  return { viewId: "diff", title: "Diff", icon: "diff" };
}

/**
 * Surface the inspector view that corresponds to the clicked tool.
 * No-op if the tool id isn't in the current agent state.
 */
export function openInspectorFromTool(toolId: string): void {
  const tool = useAgentStore.getState().toolCalls[toolId];
  if (!tool) return;

  const { viewId, title, icon } = routeForTool(tool.fn, String(tool.args));

  const ui = useUIStore.getState();
  ui.setInspectorTab(viewId as never);
  ui.setSelectedToolId(toolId);
  ui.openMainView({ id: viewId, title, icon });
}
