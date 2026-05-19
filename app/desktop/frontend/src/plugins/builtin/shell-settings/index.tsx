// Built-in plugin: fills the "app.overlay" slot with the SettingsModal. The
// modal is conditionally visible based on `useUIStore.settingsModalOpen` —
// it always renders, mountAnimation handles open/close.

import { SettingsModal } from "@/components/settings/SettingsModal";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function ShellSettings() {
  const open = useUIStore((s) => s.settingsModalOpen);
  const closeSettings = useUIStore((s) => s.closeSettings);
  return <SettingsModal open={open} onClose={closeSettings} />;
}

export default definePlugin({
  name: "lyra.builtin.shell-settings",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "settings",
      order: 0,
      component: ShellSettings,
    });
  },
});
