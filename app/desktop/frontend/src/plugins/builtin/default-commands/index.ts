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
    // ---- View: <inspector tab> — open as a main-area tab ----
    //
    // Snapshot of inspector tabs at setup time. Loaded after the inspector
    // tab builtins (see manifest ordering) so they're all registered when
    // we read them. A tab registered later won't appear in the palette
    // until reload; acceptable for built-ins.
    const inspectorTabs = Array.from(usePluginStore.getState().inspectorTabs.values())
      .map((o) => o.value)
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
    for (const tab of inspectorTabs) {
      host.commands.register({
        id: `view.open.${tab.id}`,
        label: `View: ${tab.label}`,
        icon: tab.icon,
        group: "View",
        order: 10,
        keywords: ["open", "show", tab.id],
        run: () => useUIStore.getState().openMainView({
          id: tab.id, title: tab.label, icon: tab.icon,
        }),
      });
    }

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
