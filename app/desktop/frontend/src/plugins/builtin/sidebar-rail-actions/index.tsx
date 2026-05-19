// Built-in plugin: top action buttons in the collapsed sidebar
// (New session, Search). Placeholder UI today — clicking them does
// nothing functional yet — but they hold the layout slots a user plugin
// can swap behaviour into.

import { Icon, IconButton } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";

function NewSessionBtn() {
  return (
    <IconButton variant="rail-primary" title="New session">
      <Icon name="plus" size={16} />
    </IconButton>
  );
}

function SearchBtn() {
  return (
    <IconButton variant="rail" title="Search (⌘K)">
      <Icon name="search" size={16} />
    </IconButton>
  );
}

export default definePlugin({
  name: "lyra.builtin.sidebar-rail-actions",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "new-session", order: 10, component: NewSessionBtn });
    host.sidebar.registerRailItem({ id: "search",      order: 20, component: SearchBtn });
  },
});
