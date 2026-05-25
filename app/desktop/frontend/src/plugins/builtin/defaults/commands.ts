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

import type { SidebarSession } from "@/components/sidebar/types";
import type {Disposable, ThemeAccentSpec, WorkspaceViewSpec} from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";
import {
  definePlugin,
  
  
  usePluginStore
  
} from "@/plugins/sdk";
import { useLayoutStore } from "@/state/layoutStore";
import { useSessionStore } from "@/state/sessionStore";
import { useThemeStore } from "@/state/themeStore";

// "Close the currently-focused tab" — if the user is viewing a workspace
// view in the main area, close that tab; otherwise close the active chat
// session tab. Mirrors what the close-X glyph does in ChatTopBar.
function closeFocusedTab(): void {
  const ui = useSessionStore.getState();
  if (ui.activeMainView) {
    ui.closeMainView(ui.activeMainView);
  } else if (ui.activeSessionId) {
    ui.closeTab(ui.activeSessionId);
  }
}

// "Open a new chat tab" — pick the next session that isn't already in
// the tab strip (mirrors topbar-new-tab's behavior). Returns silently
// when every available session is already open.
function openNewChatTab(): void {
  const sessions = queryClient.getQueryData<SidebarSession[]>(["sessions"]) ?? [];
  const tabIds = useSessionStore.getState().tabIds;
  const candidate = sessions.find((s) => !tabIds.includes(s.id));
  if (candidate) useSessionStore.getState().selectTab(candidate.id);
}

// "Focus the composer" — the composer textarea has a stable class name
// (set by Composer.tsx); we DOM-query rather than threading a ref through
// half the tree just for one shortcut.
function focusComposer(): void {
  document.querySelector<HTMLTextAreaElement>(".composer-input")?.focus();
}

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
      shortcut: "⌘B",
      run: () => useLayoutStore.getState().toggleSidebar(),
    });

    host.commands.register({
      id: "settings.toggle-theme",
      label: "Toggle dark/light theme",
      icon: "moon",
      group: "Theme",
      order: 0,
      shortcut: "⌘⇧L",
      run: () => useThemeStore.getState().toggleTheme(),
    });

    host.commands.register({
      id: "chat.new",
      label: "New chat tab",
      icon: "plus",
      group: "Chat",
      keywords: ["session", "tab", "open"],
      order: 0,
      shortcut: "⌘N",
      run: openNewChatTab,
    });

    host.commands.register({
      id: "chat.close-tab",
      label: "Close current tab",
      icon: "x",
      group: "Chat",
      keywords: ["dismiss"],
      order: 1,
      shortcut: "⌘W",
      run: closeFocusedTab,
    });

    host.commands.register({
      id: "composer.focus",
      label: "Focus composer",
      icon: "edit",
      group: "Composer",
      keywords: ["input", "write"],
      order: 0,
      shortcut: "⌘L",
      run: focusComposer,
    });

    // Dynamic commands: rebuild from the workspaceViews + accents registry
    // whenever either changes. Each rebuild disposes the previous batch and
    // re-registers from current state.
    let dynamic: Disposable[] = [];

    const rebuild = (views: WorkspaceViewSpec[], accents: ThemeAccentSpec[]) => {
      for (const d of dynamic) d.dispose();
      dynamic = [];
      for (const view of [...views].sort((a, b) => (a.order ?? 100) - (b.order ?? 100))) {
        dynamic.push(
          host.commands.register({
            id: `view.open.${view.id}`,
            label: `View: ${view.title}`,
            icon: view.icon,
            group: "View",
            order: 10,
            keywords: ["open", "show", view.id],
            // Hide when this view is already the focused main-area tab.
            when: `mainView != "${view.id}"`,
            run: () =>
              useSessionStore.getState().openMainView({
                id: view.id,
                title: view.title,
                icon: view.icon,
              }),
          }),
        );
      }
      for (const accent of [...accents].sort((a, b) => (a.order ?? 100) - (b.order ?? 100))) {
        dynamic.push(
          host.commands.register({
            id: `theme.accent.${accent.id}`,
            label: `Accent: ${accent.label}`,
            icon: "spark",
            group: "Theme",
            order: 10,
            run: () => useThemeStore.getState().setAccent(accent.dark),
          }),
        );
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
