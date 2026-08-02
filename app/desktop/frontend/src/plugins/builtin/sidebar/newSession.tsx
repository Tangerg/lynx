// Sidebar global actions — the Work Index opens with the app-level entry points.
//
// Three rows and the key that reaches each: starting work, finding work, and the
// unattended work a schedule is running. The shortcut hint is the point of the
// row shape — an index that teaches its own keyboard is what keeps the palette
// from being the only route to any of them.

import { SCHEDULES_PANE } from "@/plugins/builtin/settings/public/panes";
import { AgentRow } from "@/ui/agent";
import { comboGlyph } from "@/lib/combo";
import { useT } from "@/lib/i18n";
import {
  contributeWorkIndexItem,
  useWorkIndexActions,
} from "@/plugins/builtin/navigation/public/workIndex";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { usePaletteStore } from "@/plugins/builtin/command/paletteStore";
import { definePlugin } from "@/plugins/sdk";

function Shortcut({ combo }: { combo: string }) {
  return (
    <span className="font-mono text-ui-xs leading-none text-fg-faint tabular-nums">
      {comboGlyph(combo)}
    </span>
  );
}

function SidebarNewSession() {
  const t = useT();
  const actions = useWorkIndexActions();

  return (
    <div className="flex flex-col">
      <AgentRow icon="edit" onClick={actions.createSession} trailing={<Shortcut combo="Mod+N" />}>
        {t("sidebar.action.newSession")}
      </AgentRow>
      <AgentRow
        icon="search"
        onClick={() => usePaletteStore.getState().setOpen(true)}
        trailing={<Shortcut combo="Mod+K" />}
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
