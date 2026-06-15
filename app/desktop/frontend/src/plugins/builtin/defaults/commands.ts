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

import type { Disposable, ThemeAccentSpec, WorkspaceViewSpec } from "@/plugins/sdk";
import { createSession } from "@/lib/agent/useCreateSession";
import { definePlugin, lookupExtensionPoint, usePluginStore } from "@/plugins/sdk";
import { ACCENT, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";
import { t } from "@/lib/i18n";

// "Close the currently-focused tab" — if the user is viewing a workspace
// view in the main area, close that tab; otherwise close the active chat
// session tab. Mirrors what the close-X glyph does in PanelTabBar.
function closeFocusedTab(): void {
  const ui = useSessionStore.getState();
  if (ui.activeMainView) {
    ui.closeMainView(ui.activeMainView);
  } else if (ui.activeSessionId) {
    ui.closeTab(ui.activeSessionId);
  }
}

// "Open a new chat tab" — create a fresh draft session and open it.
// The previous re-open-untabbed-session approach silently no-opped when
// none was free, making ⌘N a dead key.
function openNewChatTab(): void {
  void createSession();
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
      label: t("command.toggleSidebar"),
      icon: "panel-l",
      group: "View",
      keywords: ["collapse", "expand"],
      order: 0,
      combo: "Mod+B",
      run: () => useUiStore.getState().toggleSidebar(),
    });

    host.commands.register({
      id: "settings.toggle-theme",
      label: t("command.toggleTheme"),
      icon: "moon",
      group: "Theme",
      order: 0,
      combo: "Mod+Shift+L",
      run: () => useUiStore.getState().toggleTheme(),
    });

    host.commands.register({
      id: "chat.new",
      label: t("command.newChat"),
      icon: "plus",
      group: "Chat",
      keywords: ["session", "tab", "open"],
      order: 0,
      combo: "Mod+N",
      run: openNewChatTab,
    });

    host.commands.register({
      id: "chat.close-tab",
      label: t("command.closeTab"),
      icon: "x",
      group: "Chat",
      keywords: ["dismiss"],
      order: 1,
      combo: "Mod+W",
      run: closeFocusedTab,
    });

    host.commands.register({
      id: "composer.focus",
      label: t("command.focusComposer"),
      icon: "edit",
      group: "Composer",
      keywords: ["input", "write"],
      order: 0,
      combo: "Mod+L",
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
            label: t("command.viewPrefix", { title: t(view.title) }),
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
            label: t("command.accentPrefix", { name: accent.label }),
            icon: "spark",
            group: "Theme",
            order: 10,
            run: () => useUiStore.getState().setAccent(accent.dark),
          }),
        );
      }
    };

    const snapshot = () => ({
      views: lookupExtensionPoint(WORKSPACE_VIEW),
      accents: lookupExtensionPoint(ACCENT),
    });

    // Content signature of just the inputs `rebuild` reads. `rebuild` itself
    // writes COMMAND contributions to the same `extensions` map, so a raw
    // `state.extensions !== prev.extensions` guard would re-fire on our own
    // writes and recurse forever. Skipping when the signature is unchanged
    // collapses command-only churn (incl. rebuild's own) to a no-op.
    const signature = (s: ReturnType<typeof snapshot>) =>
      [
        ...s.views.map((v) => `v:${v.id}:${v.title}:${v.icon}:${v.order ?? ""}`),
        ...s.accents.map((a) => `a:${a.id}:${a.label}:${a.order ?? ""}`),
      ].join("|");

    let lastSignature = "";
    const apply = () => {
      const next = snapshot();
      const sig = signature(next);
      if (sig === lastSignature) return;
      lastSignature = sig;
      rebuild(next.views, next.accents);
    };

    apply();
    const unsubscribe = usePluginStore.subscribe((state, prev) => {
      if (state.extensions === prev.extensions) return;
      apply();
    });

    return () => unsubscribe();
  },
});
