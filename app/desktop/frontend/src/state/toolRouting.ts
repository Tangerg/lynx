// Pure dispatcher: when a tool card is clicked, pick the right workspace
// view, set context, and promote it to the main-area tab strip.
//
// Lives outside the render tree so `ToolCard` can invoke it directly
// without anyone passing a callback down through the chat panel.

import type { ToolCall } from "@/protocol/run/viewState";
import { toolCategory } from "@/protocol/run/viewState";
import { getCurrentSessionView } from "./agentStore";
import { useSessionStore } from "./sessionStore";

interface ViewRouting {
  id: string;
  title: string;
  icon: string;
}

// toolLabel renders a multi-file edit as "N files" — not a path, so don't
// feed it to the diff view's active-file focus.
const MULTI_FILE_LABEL = /^\d+ files$/;

// Choose which workspace view to surface for the clicked tool. Routing keys
// on the WIRE identity (`tool.name` → §4.4.2 category) — `fn` is the display
// label (the command string for bash, the path for edits), never the tool
// name, so matching on it would route nothing. Each branch sets up the side
// effects the view relies on (e.g. active-file path for the diff view).
function routeForTool(tool: ToolCall): ViewRouting | null {
  const category = toolCategory(tool.name);
  if (category === "command") {
    return { id: "terminal", title: "Terminal", icon: "terminal" };
  }
  if (category === "fileEdit" || category === "read") {
    // For these categories toolLabel surfaces the file path as `fn`.
    if (tool.fn && !MULTI_FILE_LABEL.test(tool.fn)) {
      useSessionStore.getState().setActiveFile(tool.fn);
    }
    return { id: "diff", title: "Diff", icon: "diff" };
  }
  // search / webSearch / lsp_* / skill / subagent / generic have no dedicated
  // detail view — their inline preview (with its "… N more" overflow) is the
  // whole presentation. Return null so we DON'T promote an unrelated view: this
  // used to fall through to Diff, opening the git working-tree diff when the
  // user clicked "View all matches" on a grep / "View details" on an lsp call.
  return null;
}

/**
 * Surface the workspace view that corresponds to the clicked tool.
 * No-op if the tool id isn't in the current agent state.
 */
export function openViewForTool(toolId: string): void {
  const tool = getCurrentSessionView().toolCalls[toolId];
  if (!tool) return;

  const ui = useSessionStore.getState();
  ui.setSelectedToolId(toolId);
  // Open the tool's view BESIDE chat (not replacing it) — clicking a tool
  // while chatting should let you watch its diff / terminal next to the
  // conversation. routeForTool only returns splittable views (diff/terminal).
  const view = routeForTool(tool);
  if (view) ui.openMainViewBeside(view);
}
