// Sidebar global actions — the Work Index starts with app-level entry points.

import { AgentKbd, AgentRow } from "@/components/agent-studio";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { usePaletteStore } from "@/plugins/builtin/command/paletteStore";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin } from "@/plugins/sdk";

function SidebarNewSession() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col gap-px">
      <AgentRow icon="edit" className="font-medium" onClick={actions.createSession}>
        {t("sidebar.action.newSession")}
      </AgentRow>
      <AgentRow
        icon="search"
        onClick={() => usePaletteStore.getState().setOpen(true)}
        trailing={
          <span className="flex items-center gap-1">
            <AgentKbd>⌘</AgentKbd>
            <AgentKbd>K</AgentKbd>
          </span>
        }
      >
        {t("common.search")}
      </AgentRow>
      <AgentRow
        icon="history"
        onClick={() => openWorkspaceSettingsPane("schedules", t("settings.pane.schedules"))}
      >
        {t("settings.pane.schedules")}
      </AgentRow>
      <AgentRow
        icon="sparkle"
        onClick={() => openWorkspaceSettingsPane("plugins", t("settings.pane.plugins"))}
      >
        {t("settings.pane.plugins")}
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
