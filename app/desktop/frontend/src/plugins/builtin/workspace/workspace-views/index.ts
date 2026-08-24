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
export { knowledgeView } from "./knowledge";
export { agentMemoryView } from "./agentMemory";
export { skillsView } from "./skills";
export { skillLibraryView } from "./skillLibrary";
export { skillProposalsView } from "./skillProposals";
export { recipesView } from "./recipes";
export { inboxView } from "./inbox";
export { notificationsView } from "./notifications";
export { planView } from "./plan";
export { runSummaryView } from "./run-summary";
export { searchView } from "./search";
export { terminalView } from "./terminal";
export { timelineView } from "./timeline";
export { toolStatsView } from "./toolStats";
export { toolsView } from "./tools";
