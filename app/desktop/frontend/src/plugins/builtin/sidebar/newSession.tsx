// Sidebar global actions — the Work Index starts with app-level entry points.
//
// New session and schedules are the two global work entry points. Command search
// lives in the drawer header so this list can stay a calm, task-shaped index.

import { SCHEDULES_PANE } from "@/plugins/builtin/settings/public/panes";
import { AgentRow } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin } from "@/plugins/sdk";

function SidebarNewSession() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col gap-1">
      <AgentRow icon="edit" className="font-medium" onClick={actions.createSession}>
        {t("sidebar.action.newSession")}
      </AgentRow>
      <AgentRow icon="clock" onClick={() => openWorkspaceSettingsPane(SCHEDULES_PANE)}>
        {t("settings.pane.schedules")}
      </AgentRow>
    </div>
  );
}

export const sidebarNewSession = definePlugin({
  name: "lyra.builtin.sidebar-new-session",
  version: "1.0.0",
  setup({ host }) {
    contributeWorkIndexItem(host, {
      id: "new-session",
      scope: "global",
      variant: "expanded",
      order: -10,
      component: SidebarNewSession,
    });
  },
});
