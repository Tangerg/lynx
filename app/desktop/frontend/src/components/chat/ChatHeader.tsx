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

import type { ChatTab } from "./ChatTopBar";
import { useMemo } from "react";
import { useSessions } from "@/lib/queries";
import { headerTabBulkFor, useSessionStore } from "@/state/sessionStore";
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

  return (
    <ChatTopBar
      tabs={openTabs}
      viewTabs={mainViewTabs}
      activeId={headerActiveId}
      onSelectChat={selectChat}
      onCloseChat={(id) => useSessionStore.getState().closeTab(id)}
      onSelectView={(id) => useSessionStore.getState().selectMainView(id)}
      onCloseView={(id) => useSessionStore.getState().closeMainView(id)}
      bulkFor={headerTabBulkFor}
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
