// Built-in workspace views — the surfaces the user can promote into a
// main-area tab. Each lives in its own file because they're substantial
// (50–130 lines each) with their own React components; this barrel just
// re-exports the plugin specs so the manifest doesn't have to know about
// the per-view files.

export { agentDocsView } from "./agent-docs";
export { diffView } from "./diff";
export { fileView } from "./file";
export { filesView } from "./files";
export { fileTreeView } from "./filetree";
export { memoryView } from "./memory";
export { skillsView } from "./skills";
export { notificationsView } from "./notifications";
export { planView } from "./plan";
export { todosView } from "./todos";
export { runSummaryView } from "./run-summary";
export { searchView } from "./search";
export { terminalView } from "./terminal";
export { timelineView } from "./timeline";
export { toolsView } from "./tools";
