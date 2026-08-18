// Sidebar global actions — the Work Index opens with search and the app-level
// entry points.
//
// Global entry points for starting work, choosing where it runs, inspecting
// unattended work, and configuring what the agent can reach. The list below is
// the work itself, so anything that is merely a way of FINDING that work does
// not earn another row here — ⌘K is on every surface and needs no signpost in
// the one place sessions are already listed. Search is different: it scales
// beyond the visible fold while keeping the index itself compact.

import { comboGlyph } from "@/lib/combo";
import { MCP_SERVERS_PANE, SCHEDULES_PANE } from "@/plugins/builtin/settings/public/panes";
import { openSessionSearch } from "@/plugins/builtin/command/session-search/public/actions";
import { AgentRow } from "@/ui/agent";
import { Kbd } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin } from "@/plugins/sdk";

export function SidebarActions() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col gap-2">
      <AgentRow
        icon="search"
        onClick={openSessionSearch}
        aria-haspopup="dialog"
        aria-keyshortcuts="Meta+K Control+K"
        className="bg-sunken font-normal text-fg-muted hover:bg-hover hover:text-fg"
        trailing={
          <Kbd className="h-auto min-w-0 bg-transparent px-0 font-mono text-ui-2xs font-normal text-fg-faint">
            {comboGlyph("Mod+K")}
          </Kbd>
        }
      >
        {t("sessionSearch.placeholder")}
      </AgentRow>
      <div className="flex flex-col">
        {/* No combo in the trailing slot. One of three rows wore its shortcut, so the
          strip read as "this row has a property the others lack" rather than as a
          hint — and a keyboard user is not looking at the sidebar. */}
        <AgentRow icon="edit" disabled={!actions.canCreateSession} onClick={actions.createSession}>
          {t("sidebar.action.newSession")}
        </AgentRow>
        <AgentRow icon="clock" onClick={() => openWorkspaceSettingsPane(SCHEDULES_PANE)}>
          {t("settings.pane.schedules")}
        </AgentRow>
        <AgentRow icon="tool" onClick={() => openWorkspaceSettingsPane(MCP_SERVERS_PANE)}>
          {t("sidebar.action.tools")}
        </AgentRow>
      </div>
    </div>
  );
}

export const sidebarActions = definePlugin({
  name: "lyra.builtin.sidebar-actions",
  setup(ctx) {
    contributeWorkIndexItem(ctx, {
      id: "actions",
      scope: "global",
      variant: "expanded",
      order: -10,
      component: SidebarActions,
    });
  },
});
