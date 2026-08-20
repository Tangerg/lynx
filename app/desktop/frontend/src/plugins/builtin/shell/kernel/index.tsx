// Built-in plugins that fill the three primary kernel slots —
// `app.sidebar`, `app.main`, and the Settings workspace view. Each one
// stays a separate plugin so users can swap any single slot (e.g.
// replace `kernel-chat` with their own session UI) without touching the
// rest.

import { ChatPanel } from "./panel";
import { SettingsPage } from "./SettingsPage";
import { SidebarPanel } from "@/plugins/builtin/sidebar/public/SidebarPanel";
import { useSendComposerInput } from "@/plugins/builtin/chat/composer/public/sendToAgent";
import { useReconcilePersistedAgentSessions } from "@/plugins/builtin/agent/public/session";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { useDefaultChatSession } from "@/plugins/builtin/agent/public/defaultSession";
import { ComposerProjectTray } from "./panel/ProjectSelector";

function KernelChat() {
  // Drop persisted refs to sessions the backend no longer has BEFORE binding
  // the agent lifecycle, so a stale active id resolves to the welcome screen
  // instead of a dead session.
  useReconcilePersistedAgentSessions();
  // Mount the active session's agent lifecycle (subscribe + register the
  // send/stop actions); composer → agent routing goes through the
  // message-actions input bridge.
  useDefaultChatSession();
  const send = useSendComposerInput();
  return <ChatPanel onSend={send} />;
}

function KernelSidebar() {
  return <SidebarPanel />;
}

export const kernelChat = definePlugin({
  name: "lyra.builtin.kernel-chat",
  setup(ctx) {
    contributeLayout(ctx, "app.main", { id: "chat", order: 0, component: KernelChat });
    contributeLayout(ctx, "composer.overlay.top", {
      id: "project",
      order: -10,
      component: ComposerProjectTray,
    });
  },
});

export const kernelSidebar = definePlugin({
  name: "lyra.builtin.kernel-sidebar",
  setup(ctx) {
    contributeLayout(ctx, "app.sidebar", { id: "sidebar", order: 0, component: KernelSidebar });
  },
});

export const kernelSettings = definePlugin({
  name: "lyra.builtin.kernel-settings",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "settings",
      title: "settings.title",
      icon: "settings",
      // Last in the palette and never in the dock: settings is a whole-window surface.
      order: 200,
      component: SettingsPage,
    });
  },
});
