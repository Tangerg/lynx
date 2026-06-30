// Built-in plugins that fill the three primary kernel slots —
// `app.sidebar`, `app.main`, and the Settings workspace view. Each one
// stays a separate plugin so users can swap any single slot (e.g.
// replace `kernel-chat` with their own session UI) without touching the
// rest.

import { useEffect, useMemo, useRef } from "react";
import { ChatPanel } from "@/components/chat/panel";
import { SettingsPage } from "./SettingsPage";
import { SidebarPanel } from "@/components/sidebar/SidebarPanel";
import { useChatSend } from "@/lib/agent/useChatSend";
import { useSessions } from "@/lib/data/queries";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";
import { useSidebarRail } from "@/state/useSidebarRail";
import { useDefaultChatSession } from "@/state/useDefaultChatSession";

// On boot, reconcile the persisted tabs / active session against the backend's
// real session list. localStorage carries activeSessionId + tabIds across
// launches; if the runtime no longer has those sessions — the db was reset
// (`make fresh`) or a session was deleted elsewhere — a persisted ghost id
// would strand the user on a dead session whose first runs.start is rejected
// with session_not_found. Runs once, the first time sessions.list resolves;
// not-yet-graduated drafts (absent from the list by design) are kept by
// reconcileTabs, so there's no race with a session created during the run.
function useReconcilePersistedTabs() {
  const { data, isSuccess } = useSessions();
  const done = useRef(false);
  useEffect(() => {
    if (done.current || !isSuccess) return;
    done.current = true;
    useSessionStore.getState().reconcileTabs((data ?? []).map((s) => s.id));
  }, [isSuccess, data]);
}

function KernelChat() {
  // Drop persisted refs to sessions the backend no longer has BEFORE binding
  // the agent lifecycle, so a stale active id resolves to the welcome screen
  // instead of a dead session.
  useReconcilePersistedTabs();
  // Mount the active session's agent lifecycle (subscribe + register the
  // send/stop actions); the send routing itself goes through useChatSend.
  useDefaultChatSession();
  const send = useChatSend();
  return <ChatPanel onSend={send} />;
}

function KernelSidebar() {
  const railed = useSidebarRail();
  const activeSession = useSessionStore((s) => s.activeSessionId);
  const selectTab = useSessionStore((s) => s.selectTab);
  const toggleSidebar = useUiStore((s) => s.toggleSidebar);

  // Only the rail view still needs the sessions list (the expanded view
  // gets it via the plugin-contributed sidebar sections). Drafts are hidden
  // until their first message graduates them.
  const { data = [] } = useSessions();
  const draftIds = useSessionStore((s) => s.draftSessionIds);
  const sessions = useMemo(() => data.filter((s) => !draftIds.has(s.id)), [data, draftIds]);

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
