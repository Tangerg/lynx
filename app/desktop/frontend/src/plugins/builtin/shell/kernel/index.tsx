// Built-in plugins that fill the three primary kernel slots —
// `app.sidebar`, `app.main`, and the Settings workspace view. Each one
// stays a separate plugin so users can swap any single slot (e.g.
// replace `kernel-chat` with their own session UI) without touching the
// rest.

import { ChatPanel } from "@/components/chat/panel";
import { SettingsPage } from "./SettingsPage";
import { SidebarPanel } from "@/components/sidebar/SidebarPanel";
import { useSendComposerInput } from "@/plugins/builtin/chat/composer/public/sendToAgent";
import {
  useActiveSessionId,
  useReconcilePersistedAgentSessions,
  useSelectAgentSession,
  useVisibleAgentSessions,
} from "@/plugins/builtin/agent/public/session";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { useUiStore } from "@/state/uiStore";
import { useSidebarRail } from "@/plugins/builtin/workspace/public/sidebarRail";
import { useDefaultChatSession } from "@/plugins/builtin/agent/public/defaultSession";

function KernelChat() {
  // Drop persisted refs to sessions the backend no longer has BEFORE binding
  // the agent lifecycle, so a stale active id resolves to the welcome screen
  // instead of a dead session.
  useReconcilePersistedAgentSessions();
  // Mount the active session's agent lifecycle (subscribe + register the
  // send/stop actions); composer → agent routing goes through the chat workbench.
  useDefaultChatSession();
  const send = useSendComposerInput();
  return <ChatPanel onSend={send} />;
}

function KernelSidebar() {
  const railed = useSidebarRail();
  const activeSession = useActiveSessionId();
  const selectTab = useSelectAgentSession();
  const toggleSidebar = useUiStore((s) => s.toggleSidebar);
  const sessions = useVisibleAgentSessions();

  return (
    <SidebarPanel
      sessions={sessions}
      activeSessionId={activeSession}
      onSelect={selectTab}
      rail={railed}
      onToggleRail={toggleSidebar}
    />
  );
}

export const kernelChat = definePlugin({
  name: "lyra.builtin.kernel-chat",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.main", { id: "chat", order: 0, component: KernelChat });
  },
});

export const kernelSidebar = definePlugin({
  name: "lyra.builtin.kernel-sidebar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.sidebar", { id: "sidebar", order: 0, component: KernelSidebar });
  },
});

export const kernelSettings = definePlugin({
  name: "lyra.builtin.kernel-settings",
  version: "1.0.0",
  setup({ host }) {
    host.extensions.contribute(WORKSPACE_VIEW, {
      id: "settings",
      title: "settings.title",
      icon: "settings",
      openByDefault: false,
      component: SettingsPage,
    });
  },
});
