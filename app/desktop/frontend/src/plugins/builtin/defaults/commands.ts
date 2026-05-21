// Built-in plugin: a starter set of palette commands.
//
// Static commands (toggle sidebar / toggle theme) register once. The
// dynamic ones — "View: <X>" per workspace view and "Accent: <X>" per
// theme accent — track the registry reactively: any time a plugin
// registers or unloads a view / accent, the command list rebuilds.
//
// The reactive approach is why this plugin no longer needs `requires`:
// it doesn't matter whether contributors load before or after — the
// subscription catches up either way.

import {
  definePlugin,
  usePluginStore,
  type Disposable,
  type ThemeAccentSpec,
  type WorkspaceViewSpec,
} from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

export const defaultCommands = definePlugin({
  name: "lyra.builtin.default-commands",
  version: "1.0.0",
  setup({ host }) {
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
      id: "settings.toggle-theme",
      label: "Toggle dark/light theme",
      icon: "moon",
      group: "Theme",
      order: 0,
      run: () => useUIStore.getState().toggleTheme(),
    });

    // Dynamic commands: rebuild from the workspaceViews + accents registry
    // whenever either changes. Each rebuild disposes the previous batch and
    // re-registers from current state.
    let dynamic: Disposable[] = [];

    const rebuild = (views: WorkspaceViewSpec[], accents: ThemeAccentSpec[]) => {
      for (const d of dynamic) d.dispose();
      dynamic = [];
      for (const view of [...views].sort((a, b) => (a.order ?? 100) - (b.order ?? 100))) {
        dynamic.push(host.commands.register({
          id: `view.open.${view.id}`,
          label: `View: ${view.title}`,
          icon: view.icon,
          group: "View",
          order: 10,
          keywords: ["open", "show", view.id],
          // Hide when this view is already the focused main-area tab.
          when: `mainView != "${view.id}"`,
          run: () => useUIStore.getState().openMainView({
            id: view.id, title: view.title, icon: view.icon,
          }),
        }));
      }
      for (const accent of [...accents].sort((a, b) => (a.order ?? 100) - (b.order ?? 100))) {
        dynamic.push(host.commands.register({
          id: `theme.accent.${accent.id}`,
          label: `Accent: ${accent.label}`,
          icon: "spark",
          group: "Theme",
          order: 10,
          run: () => useUIStore.getState().setAccent(accent.dark),
        }));
      }
    };

    const snapshot = () => {
      const s = usePluginStore.getState();
      return {
        views: Array.from(s.workspaceViews.values()).map((o) => o.value),
        accents: Array.from(s.accents.values()).map((o) => o.value),
      };
    };

    const initial = snapshot();
    rebuild(initial.views, initial.accents);

    const unsubscribe = usePluginStore.subscribe((state, prev) => {
      if (state.workspaceViews === prev.workspaceViews && state.accents === prev.accents) return;
      const next = snapshot();
      rebuild(next.views, next.accents);
    });

    return () => unsubscribe();
  },
});
