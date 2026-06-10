// PanelHeader — the tab strip at the top of the chat panel.
//
// Subscribes to the bits of session state that drive the strip (open
// tab ids, main-view tabs, active selection) and computes the
// presentational `openTabs` list from the sessions query. Hands the
// shaped data + callbacks to PanelTabBar.
//
// Naming: the strip mixes chat-session tabs with workspace-view tabs
// (Settings included). "PanelHeader" sidesteps the "view" overload
// that already names workspace views in this codebase.

import type { ChatTab } from "./PanelTabBar";
import { useMemo } from "react";
import { useSessions } from "@/lib/data/queries";
import { headerTabCloseActionsFor, useSessionStore } from "@/state/sessionStore";
import { PanelTabBar } from "./PanelTabBar";

export function PanelHeader() {
  const { data: sessions = [] } = useSessions();
  const tabIds = useSessionStore((s) => s.tabIds);
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const mainViewTabs = useSessionStore((s) => s.mainViewTabs);
  const activeMainView = useSessionStore((s) => s.activeMainView);

  // One tab per id in tabIds, no exceptions. If `sessions` doesn't
  // (yet / any more) carry metadata for an id — e.g. the sessions
  // query is mid-refetch, or the user just opened a tab whose row
  // hasn't landed in the cache yet — fall back to a placeholder with
  // the id as the title. The previous `filter(Boolean)` silently
  // dropped such tabs, producing the "I clicked but no tab appeared"
  // bug; the strip is supposed to reflect tabIds 1:1.
  const openTabs: ChatTab[] = useMemo(
    () =>
      tabIds.map((id) => {
        const found = sessions.find((s) => s.id === id);
        return found
          ? { id: found.id, title: found.title, status: found.status }
          : { id, title: id, status: "idle" as const };
      }),
    [tabIds, sessions],
  );

  const headerActiveId = activeMainView ?? activeSessionId;

  return (
    <PanelTabBar
      tabs={openTabs}
      viewTabs={mainViewTabs}
      activeId={headerActiveId}
      // selectTab itself clears activeMainView (sessionStore) — the
      // return-to-chat behavior is uniform across every entry point
      // (this strip, sidebar rows, Cmd+1..9, ⌘N).
      onSelectChat={(id) => useSessionStore.getState().selectTab(id)}
      onCloseChat={(id) => useSessionStore.getState().closeTab(id)}
      onSelectView={(id) => useSessionStore.getState().selectMainView(id)}
      onCloseView={(id) => useSessionStore.getState().closeMainView(id)}
      closeActionsFor={headerTabCloseActionsFor}
    />
  );
}
