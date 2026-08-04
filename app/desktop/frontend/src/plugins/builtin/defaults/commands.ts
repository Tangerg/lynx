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

import type { AccentSpec, Disposable, WorkspaceViewSpec } from "@/plugins/sdk";
import { toggleThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import { closeActiveAgentSession, createSession } from "@/plugins/builtin/agent/public/session";
import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  openWorkspaceView,
  openWorkspaceViewInDock,
} from "@/plugins/builtin/workspace/public/navigation";
import { definePlugin, lookupExtensionPoint, usePluginStore } from "@/plugins/sdk";
import { ACCENT, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { useUiStore } from "@/state/uiStore";
import { navigator } from "@/lib/navigation";
import {
  defaultAccentCommands,
  defaultStaticCommands,
  defaultWorkspaceViewCommands,
} from "./application/defaultContributions";

// Close the surface the user is looking at, innermost first: a full view, then
// the active dock tab, then the session itself.
function closeFocusedSurface(): void {
  if (closeActiveWorkspaceView()) return;
  if (closeActiveWorkspaceDockView()) return;
  closeActiveAgentSession();
}

// Open a fresh session — creating one only if the user isn't already looking at
// one — and put the caret in the composer either way.
function openNewChatSession(): void {
  void createSession().then(() => focusComposer());
}

export const defaultCommands = definePlugin({
  name: "lyra.builtin.default-commands",
  version: "1.0.0",
  setup({ host }) {
    for (const command of defaultStaticCommands({
      toggleSidebar: () => useUiStore.getState().toggleSidebar(),
      toggleTheme: toggleThemeScheme,
      newChat: openNewChatSession,
      closeFocused: closeFocusedSurface,
      focusComposer: () => focusComposer(),
      historyBack: () => navigator().back(),
      historyForward: () => navigator().forward(),
    })) {
      host.commands.register(command);
    }

    // Dynamic commands: rebuild from the workspaceViews + accents registry
    // whenever either changes. Each rebuild disposes the previous batch and
    // re-registers from current state.
    let dynamic: Disposable[] = [];

    const rebuild = (views: WorkspaceViewSpec[], accents: AccentSpec[]) => {
      for (const d of dynamic) d.dispose();
      dynamic = [];
      for (const command of defaultWorkspaceViewCommands(views, {
        openInDock: openWorkspaceViewInDock,
        openFull: openWorkspaceView,
      })) {
        dynamic.push(host.commands.register(command));
      }
      for (const command of defaultAccentCommands(accents, (accent) =>
        useUiStore.getState().setAccent(accent),
      )) {
        dynamic.push(host.commands.register(command));
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
