// Built-in plugin: the search box at the top of the expanded sidebar.
//
// Currently a placeholder input — there's no global "search files /
// commands" implementation yet. Clicking the ⌘K kbd hint opens the
// command palette (which IS implemented). A future plugin that wires a
// real local-files index can replace this contribution.

import { Icon } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";

function SidebarSearch() {
  return (
    <div className="side-search">
      <div className="side-search-icon"><Icon name="search" size={14} /></div>
      <input placeholder="Search · files · commands" />
      {/* search-kbd is absolutely positioned inside .side-search, so it
          needs that exact class — not the generic .kbd primitive. */}
      <span className="search-kbd">⌘K</span>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.sidebar-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.search", {
      id: "default",
      order: 0,
      component: SidebarSearch,
    });
  },
});
