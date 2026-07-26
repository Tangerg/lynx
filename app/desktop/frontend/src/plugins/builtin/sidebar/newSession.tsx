// Sidebar global actions — the Work Index starts with app-level entry points.
//
// Two of them are about work: start a session, or find one. What used to sit here
// beside them were two deep links into Settings (schedules, plugins), which read
// as peers of "New session" and then replaced the whole window. Only schedules
// survives, because a scheduled run is work this index is about; plugins live
// where the rest of the settings do.
//
// The palette row says what it opens. Labelled "Search" with a magnifier, it
// promised the thing ⌘F does (find in this conversation) and the thing the
// workspace's Search view does (grep the project), and delivered neither.

import { SCHEDULES_PANE } from "@/plugins/builtin/settings/public/panes";
import { Kbd } from "@/ui";
import { AgentRow } from "@/ui/agent";
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
    <div className="flex flex-col gap-0.5">
      <AgentRow icon="edit" className="font-medium" onClick={actions.createSession}>
        {t("sidebar.action.newSession")}
      </AgentRow>
      <AgentRow
        icon="command"
        onClick={() => usePaletteStore.getState().setOpen(true)}
        trailing={
          <span className="flex items-center gap-1">
            <Kbd>⌘</Kbd>
            <Kbd>K</Kbd>
          </span>
        }
      >
        {t("command.openPalette")}
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
