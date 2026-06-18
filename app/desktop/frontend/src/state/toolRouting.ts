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

// The only categories with a dedicated workspace view: command → terminal,
// fileEdit / read → diff. Everything else (search / glob / webSearch / lsp_* /
// skill / subagent / generic) is fully presented by its inline preview — it has
// no "open the full view" target, so the preview's foot button is hidden for it
// (a foot that promoted an unrelated view, or no-op'd, read as a dead button).
const VIEWED_CATEGORIES = new Set(["command", "fileEdit", "read"]);

/** Whether the tool has a workspace view to open — the gate for showing the
 *  preview's "view details" foot. Pure (no side effects), unlike routeForTool. */
export function hasToolView(tool: ToolCall): boolean {
  return VIEWED_CATEGORIES.has(toolCategory(tool.name));
}

// Choose which workspace view to surface for the clicked tool. Routing keys
// on the WIRE identity (`tool.name` → §4.4.2 category) — `fn` is the display
// label (the command string for bash, the path for edits), never the tool
// name, so matching on it would route nothing. Each branch sets up the side
// effects the view relies on (e.g. active-file path for the diff view).
function routeForTool(tool: ToolCall): ViewRouting | null {
  if (!hasToolView(tool)) return null;
  if (toolCategory(tool.name) === "command") {
    return { id: "terminal", title: "workspace.view.title.terminal", icon: "terminal" };
  }
  // fileEdit | read — toolLabel surfaces the file path as `fn`.
  if (tool.fn && !MULTI_FILE_LABEL.test(tool.fn)) {
    useSessionStore.getState().setActiveFile(tool.fn);
  }
  return { id: "diff", title: "workspace.view.title.diff", icon: "diff" };
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
