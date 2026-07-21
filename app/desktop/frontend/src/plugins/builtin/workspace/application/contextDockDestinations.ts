import type { ContextDockDestinationSpec } from "@/plugins/sdk";

// Which workspace views appear in the Context Dock, and under which scope.
// title / icon / component come from each view's own WorkspaceViewSpec (see
// workspace-views/), joined at launch time — a test guards that every viewId
// here resolves to a registered view.
export const builtinContextDockDestinations: ContextDockDestinationSpec[] = [
  { viewId: "search", scope: "workspace", order: 10 },
  { viewId: "explorer", scope: "workspace", order: 20 },
  { viewId: "files", scope: "workspace", order: 30 },
  { viewId: "diff", scope: "workspace", order: 40 },
  { viewId: "codebase", scope: "workspace", order: 50 },
  { viewId: "tools", scope: "workspace", order: 60 },
  { viewId: "skills", scope: "workspace", order: 70 },
  { viewId: "skill-drafts", scope: "workspace", order: 75 },
  { viewId: "recipes", scope: "workspace", order: 80 },
  { viewId: "memory", scope: "workspace", order: 90 },
  { viewId: "agent-memory", scope: "workspace", order: 95 },
  { viewId: "agent-docs", scope: "workspace", order: 100 },
  { viewId: "plan", scope: "run", order: 110 },
  { viewId: "todos", scope: "run", order: 120 },
  { viewId: "timeline", scope: "session", order: 130 },
  { viewId: "terminal", scope: "workspace", order: 140 },
];
