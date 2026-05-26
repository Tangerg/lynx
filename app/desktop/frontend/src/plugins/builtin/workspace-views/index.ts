// Built-in workspace views — the surfaces the user can promote into a
// main-area tab. Each lives in its own file because they're substantial
// (50–130 lines each) with their own React components; this barrel just
// re-exports the plugin specs so the manifest doesn't have to know about
// the per-view files.

export { diffView } from "./diff";
export { filesView } from "./files";
export { notificationsView } from "./notifications";
export { planView } from "./plan";
export { terminalView } from "./terminal";
export { timelineView } from "./timeline";
export { toolsView } from "./tools";
