// Built-in plugin: the palette's command set.
//
// Eight static application actions. Workspace destinations and theme accents
// belong to their own selection surfaces rather than command registration.
//
// Both had a better home already. Panels belong to the Context Dock's own picker,
// which groups them by scope and shows which are open; the palette reaches the
// same list directly and gates it behind a query, the way it already gated
// sessions. Accents belong to Appearance settings — a colour is a preference, not
// an action, and "Blue" as a top-level command said nothing about what it did.
//
// Losing the dynamic half also loses the machinery it needed: a registry
// subscription, a content signature to stop it re-firing on its own writes, and a
// dispose-and-re-register cycle. None of that has anything left to serve.

import { toggleThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import {
  closeActiveAgentSession,
  createSession,
  getActiveSessionId,
} from "@/plugins/builtin/agent/public/session";
import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  toggleWorkspaceDock,
} from "@/plugins/builtin/workspace/public/navigation";
import { COMMAND, definePlugin } from "@/plugins/sdk";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { useUiStore } from "@/state/uiStore";
import { navigator } from "@/lib/navigation";
import { defaultStaticCommands } from "./application/defaultContributions";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

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
  if (!runtimeCommandsAvailable()) return;
  if (!getActiveSessionId()) {
    focusComposer();
    return;
  }
  void createSession().then((sessionId) => {
    if (sessionId) focusComposer();
  });
}

export const defaultCommands = definePlugin({
  name: "lyra.builtin.default-commands",
  setup(ctx) {
    for (const command of defaultStaticCommands({
      toggleSidebar: () => useUiStore.getState().toggleSidebar(),
      toggleDock: toggleWorkspaceDock,
      toggleTheme: toggleThemeScheme,
      newChat: openNewChatSession,
      closeFocused: closeFocusedSurface,
      focusComposer: () => focusComposer(),
      historyBack: () => navigator().back(),
      historyForward: () => navigator().forward(),
    })) {
      ctx.contribute(COMMAND, command);
    }
  },
});
