import type { ContextDockDestinationSpec } from "@/plugins/sdk";

// Which workspace views appear in the Context Dock, and under which scope.
// title / icon / component come from each view's own WorkspaceViewSpec (see
// workspace-views/), joined at launch time — a test guards that every viewId
// here resolves to a registered view.
//
// Every registered view a user can navigate to is listed so the add-panel menu
// remains the discoverable entry point for the whole right workspace.
export const builtinContextDockDestinations: ContextDockDestinationSpec[] = [
  { viewId: "search", scope: "workspace", order: 10 },
  // Not session-scoped: the queue is every session's, which is the whole reason
  // it exists — a session-scoped inbox could only ever tell you about the session
  // you were already looking at.
  { viewId: "inbox", scope: "workspace", order: 15 },
  { viewId: "explorer", scope: "workspace", order: 20 },
  { viewId: "file", scope: "workspace", order: 25 },
  { viewId: "files", scope: "workspace", order: 30 },
  { viewId: "diff", scope: "workspace", order: 40 },
  { viewId: "terminal", scope: "workspace", order: 60 },
  { viewId: "tools", scope: "workspace", order: 70 },
  { viewId: "skills", scope: "workspace", order: 80 },
  { viewId: "skill-proposals", scope: "workspace", order: 85 },
  { viewId: "skill-library", scope: "workspace", order: 90 },
  { viewId: "recipes", scope: "workspace", order: 95 },
  { viewId: "knowledge", scope: "workspace", order: 100 },
  { viewId: "agent-memory", scope: "workspace", order: 105 },
  { viewId: "agent-docs", scope: "workspace", order: 110 },
  { viewId: "diagnostics", scope: "workspace", order: 115 },
  { viewId: "plan", scope: "session", order: 120 },
  { viewId: "run-summary", scope: "run", order: 130 },
  { viewId: "timeline", scope: "session", order: 140 },
  { viewId: "notifications", scope: "session", order: 145 },
  { viewId: "tool-stats", scope: "session", order: 150 },
];
