// Sidebar global actions — the Work Index opens with the app-level entry points.
//
// Three rows, and none of them is a session: starting work, the unattended work
// a schedule is running, and what the agent can reach. The list below them is
// the work itself, so anything that is merely a way of FINDING that work does
// not earn a row here — ⌘K is on every surface and needs no signpost in the one
// place the sessions are already listed.

import { MCP_SERVERS_PANE, SCHEDULES_PANE } from "@/plugins/builtin/settings/public/panes";
import { AgentRow } from "@/ui/agent";
import { comboGlyph } from "@/lib/combo";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin } from "@/plugins/sdk";

function Shortcut({ combo }: { combo: string }) {
  return (
    <span className="font-mono text-ui-xs leading-none text-fg-faint tabular-nums">
      {comboGlyph(combo)}
    </span>
  );
}

function SidebarActions() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col">
      <AgentRow icon="edit" onClick={actions.createSession} trailing={<Shortcut combo="Mod+N" />}>
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
