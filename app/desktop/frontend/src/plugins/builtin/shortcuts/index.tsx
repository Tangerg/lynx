// Built-in plugin: mounts ShortcutsProvider on the overlay slot + registers
// the one default shortcut we ship with — Escape closes the settings modal.
//
// More shortcuts can be added by other plugins; this just wires the host
// listener and the single behaviour the previous shell hardcoded inside
// SettingsModal's local useEffect.

import { ShortcutsProvider } from "@/plugins/ShortcutsProvider";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

export default definePlugin({
  name: "lyra.builtin.shortcuts",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "shortcuts-provider",
      // Render before toaster (which is order 100) — order here doesn't
      // matter functionally (the component returns null) but keeping it
      // first puts the side-effect mount before any visible overlays.
      order: 50,
      component: ShortcutsProvider,
    });

    host.shortcuts.register({
      key: "Escape",
      description: "Close the settings modal",
      handler: () => {
        // Read state imperatively so the handler doesn't capture stale UI
        // state from when the plugin loaded.
        const ui = useUIStore.getState();
        if (ui.settingsModalOpen) ui.closeSettings();
      },
    });
  },
});
