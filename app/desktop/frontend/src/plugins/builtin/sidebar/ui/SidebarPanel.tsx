import { SidebarExpanded } from "./SidebarExpanded";

// The work-index sidebar. There is no collapsed "rail" variant — collapsing
// hides the sidebar entirely (the shell drops the column), Codex-style.
export function SidebarPanel() {
  return <SidebarExpanded />;
}
