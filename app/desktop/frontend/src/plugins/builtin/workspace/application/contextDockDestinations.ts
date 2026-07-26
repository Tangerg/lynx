import type { ContextDockDestinationSpec } from "@/plugins/sdk";

// Which workspace views appear in the Context Dock, and under which scope.
// title / icon / component come from each view's own WorkspaceViewSpec (see
// workspace-views/), joined at launch time — a test guards that every viewId
// here resolves to a registered view.
//
// Every registered view a user can navigate to is listed: the launcher is the
// hub, so a view missing from it is a view reachable only by knowing its name in
// the command palette. `file` is here too — pinned, because it is what a file
// reference in the transcript opens into.
export const builtinContextDockDestinations: ContextDockDestinationSpec[] = [
  { viewId: "search", scope: "workspace", order: 10 },
  { viewId: "explorer", scope: "workspace", order: 20 },
  { viewId: "file", scope: "workspace", order: 25, pinned: true },
  { viewId: "files", scope: "workspace", order: 30 },
  { viewId: "diff", scope: "workspace", order: 40, pinned: true },
  { viewId: "codebase", scope: "workspace", order: 50, pinned: true },
  { viewId: "terminal", scope: "workspace", order: 60, pinned: true },
  { viewId: "tools", scope: "workspace", order: 70 },
  { viewId: "skills", scope: "workspace", order: 80 },
  { viewId: "skill-drafts", scope: "workspace", order: 85 },
  { viewId: "skill-library", scope: "workspace", order: 90 },
  { viewId: "recipes", scope: "workspace", order: 95 },
  { viewId: "memory", scope: "workspace", order: 100 },
  { viewId: "agent-memory", scope: "workspace", order: 105 },
  { viewId: "agent-docs", scope: "workspace", order: 110 },
  { viewId: "diagnostics", scope: "workspace", order: 115 },
  { viewId: "plan", scope: "run", order: 120 },
  { viewId: "todos", scope: "run", order: 125 },
  { viewId: "run-summary", scope: "run", order: 130 },
  { viewId: "timeline", scope: "session", order: 140 },
  { viewId: "notifications", scope: "session", order: 145 },
];
