// Built-in plugins that fill the three primary kernel slots —
// `app.sidebar`, `app.main`, and the Settings workspace view. Each one
// stays a separate plugin so users can swap any single slot (e.g.
// replace `kernel-chat` with their own session UI) without touching the
// rest.

import { ChatPanel } from "@/components/chat/ChatPanel";
import { SettingsPage } from "@/components/settings/SettingsPage";
import { SidebarPanel } from "@/components/sidebar/SidebarPanel";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useDefaultChatSession } from "@/state/useDefaultChatSession";
import { useLayoutStore } from "@/state/layoutStore";
import { useSessionStore } from "@/state/sessionStore";

function KernelChat() {
  const session = useDefaultChatSession();
  // Clearing the textarea after submit is owned by `submitComposer` so
  // this callback only has to forward into the live agent.
  return <ChatPanel onSend={session.send} />;
}

function KernelSidebar() {
  const sidebarRail = useLayoutStore((s) => s.sidebarRail);
  const activeSession = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);
  const toggleSidebar = useLayoutStore((s) => s.toggleSidebar);

  // Only the rail view still needs the sessions list (the expanded view
  // gets it via the plugin-contributed sidebar sections).
  const { data: sessions = [] } = useSessions();

  return (
    <SidebarPanel
      sessions={sessions}
      activeSessionId={activeSession}
      onSelect={selectTab}
      rail={sidebarRail}
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
    host.workspace.registerView({
      id: "settings",
      title: "Settings",
      icon: "settings",
      openByDefault: false,
      component: SettingsPage,
    });
  },
});
