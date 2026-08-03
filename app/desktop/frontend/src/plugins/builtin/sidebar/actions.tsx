// Sidebar global actions — the Work Index opens with the app-level entry points.
//
// Three rows, and none of them is a session: starting work, the unattended work
// a schedule is running, and what the agent can reach. The list below them is
// the work itself, so anything that is merely a way of FINDING that work does
// not earn a row here — ⌘K is on every surface and needs no signpost in the one
// place the sessions are already listed.

import { MCP_SERVERS_PANE, SCHEDULES_PANE } from "@/plugins/builtin/settings/public/panes";
import { AgentRow } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin } from "@/plugins/sdk";

function SidebarActions() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col">
      {/* No combo in the trailing slot. One of three rows wore its shortcut, so the
          strip read as "this row has a property the others lack" rather than as a
          hint — and a keyboard user is not looking at the sidebar. */}
      <AgentRow icon="edit" onClick={actions.createSession}>
        {t("sidebar.action.newSession")}
      </AgentRow>
      <AgentRow icon="clock" onClick={() => openWorkspaceSettingsPane(SCHEDULES_PANE)}>
        {t("settings.pane.schedules")}
      </AgentRow>
      <AgentRow icon="tool" onClick={() => openWorkspaceSettingsPane(MCP_SERVERS_PANE)}>
        {t("sidebar.action.tools")}
      </AgentRow>
    </div>
  );
}

export const sidebarActions = definePlugin({
  name: "lyra.builtin.sidebar-actions",
  version: "1.0.0",
  setup({ host }) {
    contributeWorkIndexItem(host, {
      id: "actions",
      scope: "global",
      variant: "expanded",
      order: -10,
      component: SidebarActions,
    });
  },
});
