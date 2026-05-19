// Built-in plugin: a starter set of palette commands.
//
// These give the empty Cmd+K palette something to do out of the box — view
// toggles, settings, and accent shortcuts. User plugins can register more.
//
// Order matters in the manifest: this plugin must load AFTER
// `lyra.builtin.default-themes` so the accent snapshot below picks up the
// built-in accents.

import { definePlugin, usePluginStore } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

export default definePlugin({
  name: "lyra.builtin.default-commands",
  version: "1.0.0",
  setup({ host }) {
    // ---- View toggles ----
    host.commands.register({
      id: "view.toggle-sidebar",
      label: "Toggle sidebar rail",
      icon: "panel-l",
      group: "View",
      keywords: ["collapse", "expand"],
      order: 0,
      run: () => useUIStore.getState().toggleSidebar(),
    });
    host.commands.register({
      id: "view.toggle-inspector",
      label: "Toggle inspector panel",
      icon: "panel-r",
      group: "View",
      keywords: ["right", "drawer"],
      order: 1,
      run: () => useUIStore.getState().toggleInspector(),
    });

    // ---- Settings ----
    host.commands.register({
      id: "settings.open",
      label: "Open settings",
      icon: "settings",
      group: "Settings",
      order: 0,
      run: () => useUIStore.getState().openSettings(),
    });
    host.commands.register({
      id: "settings.toggle-theme",
      label: "Toggle dark/light theme",
      icon: "moon",
      group: "Theme",
      order: 0,
      run: () => useUIStore.getState().toggleTheme(),
    });

    // ---- Per-accent shortcuts ----
    //
    // Snapshot reads — commands are registered at setup time, so a new
    // accent registered later won't auto-appear. For a fully reactive
    // version we'd subscribe to the accents map and register/dispose on
    // changes; this snapshot is enough for the built-in palette.
    const accents = Array.from(usePluginStore.getState().accents.values())
      .map((o) => o.value)
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));

    for (const accent of accents) {
      host.commands.register({
        id: `theme.accent.${accent.id}`,
        label: `Accent: ${accent.label}`,
        icon: "spark",
        group: "Theme",
        order: 10,
        run: () => useUIStore.getState().setAccent(accent.dark),
      });
    }
  },
});
