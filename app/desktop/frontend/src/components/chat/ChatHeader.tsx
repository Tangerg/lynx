// ChatHeader — the tab strip at the top of the chat panel.
//
// Subscribes to the bits of session state that drive the strip (open
// tab ids, main-view tabs, active selection) and computes the
// presentational `openTabs` list from the sessions query. Hands the
// shaped data + callbacks to ChatTopBar.
//
// Owning this here keeps ChatPanel decoupled from the strip's data
// wiring — adding a chip / changing the selection model doesn't
// ripple through to the parent.

import type { ChatTab, TabBulkActions } from "./ChatTopBar";
import { useMemo } from "react";
import { useSessions } from "@/lib/queries";
import { useSessionStore } from "@/state/sessionStore";
import { ChatTopBar } from "./ChatTopBar";

export function ChatHeader() {
  const { data: sessions = [] } = useSessions();
  const tabIds = useSessionStore((s) => s.tabIds);
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const mainViewTabs = useSessionStore((s) => s.mainViewTabs);
  const activeMainView = useSessionStore((s) => s.activeMainView);

  const openTabs: ChatTab[] = useMemo(
    () =>
      tabIds
        .map((id) => sessions.find((s) => s.id === id))
        .filter((s): s is (typeof sessions)[number] => Boolean(s))
        .map((s) => ({ id: s.id, title: s.title, status: s.status })),
    [tabIds, sessions],
  );

  const headerActiveId = activeMainView ?? activeSessionId;

  // Bulk-close handler bundles. Pulled out of the store on demand
  // (rather than subscribed) to avoid re-rendering the strip when
  // unrelated session-store state changes.
  const chatBulk: TabBulkActions = {
    onCloseOthers: (id) => useSessionStore.getState().closeOtherTabs(id),
    onCloseLeft: (id) => useSessionStore.getState().closeTabsLeftOf(id),
    onCloseRight: (id) => useSessionStore.getState().closeTabsRightOf(id),
    onCloseAll: () => useSessionStore.getState().closeAllTabs(),
  };
  const viewBulk: TabBulkActions = {
    onCloseOthers: (id) => useSessionStore.getState().closeOtherMainViews(id),
    onCloseLeft: (id) => useSessionStore.getState().closeMainViewsLeftOf(id),
    onCloseRight: (id) => useSessionStore.getState().closeMainViewsRightOf(id),
    onCloseAll: () => useSessionStore.getState().closeAllMainViews(),
  };

  return (
    <ChatTopBar
      tabs={openTabs}
      viewTabs={mainViewTabs}
      activeId={headerActiveId}
      onSelectChat={selectChat}
      onCloseChat={(id) => useSessionStore.getState().closeTab(id)}
      onSelectView={(id) => useSessionStore.getState().selectMainView(id)}
      onCloseView={(id) => useSessionStore.getState().closeMainView(id)}
      chatBulk={chatBulk}
      viewBulk={viewBulk}
    />
  );
}

// Switching to a chat session has to clear `activeMainView` first so the
// workspace-view tab doesn't stay focused.
function selectChat(id: string) {
  const ui = useSessionStore.getState();
  ui.selectChat();
  ui.selectTab(id);
}
