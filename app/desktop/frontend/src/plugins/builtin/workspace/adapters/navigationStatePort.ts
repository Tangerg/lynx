// Which surface fills the content card, and which settings pane is open, are
// the user's location — they come from the Navigator, so history holds them.
// What the dock has OPEN, and the per-view state inside it, is memory the
// location doesn't describe: that stays in the store.
import { useUiStore } from "@/state/uiStore";
import { useContextDockStore } from "@/state/contextDockStore";
import { navigator } from "@/lib/navigation";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";

/** Leaving a promoted view returns to the chat, which is the only other surface. */
function selectChat(): void {
  navigator().go({ view: null });
}

export function installWorkspaceNavigationPort(): () => void {
  return configureWorkspaceNavigationPort({
    useActiveViewId: () => navigator().use((location) => location.view),
    useDock: () => ({
      open: useContextDockStore((state) => state.dockOpen),
      viewIds: useContextDockStore((state) => state.dockViewIds),
      activeViewId: useContextDockStore((state) => state.activeDockViewId),
    }),
    useActiveFile: () => useContextDockStore((state) => state.activeFile),
    useFileViewer: () => useContextDockStore((state) => state.fileViewer),
    useSettingsPaneTarget: () => navigator().use((location) => location.settings),
    useExpandedToolIds: () => useContextDockStore((state) => state.expandedToolIds),
    useSelectTool: () => useContextDockStore((state) => state.setSelectedToolId),
    useToggleTool: () => useContextDockStore((state) => state.toggleExpandedTool),
    // The drawer follows the user's preference and nothing else. It used to be
    // forced collapsed while a dock view was open, which left the toggle looking
    // enabled while doing nothing — the dock is a resizable column, so the room
    // it needs is the user's to give.
    useSidebarDrawer: () => ({
      collapsed: useUiStore((state) => state.sidebarCollapsed),
      toggle: useUiStore((state) => state.toggleSidebar),
    }),
    useSidebarWidth: () => ({
      width: useUiStore((state) => state.sidebarWidth),
      setWidth: useUiStore((state) => state.setSidebarWidth),
    }),
    useDockWidth: () => {
      const setDockWidth = useUiStore((state) => state.setDockWidth);
      return {
        width: useUiStore((state) => state.dockWidth),
        setWidth: setDockWidth,
      };
    },
    selectChat,
    // Taking the whole card leaves the dock's own selection alone: closing the
    // full view brings back whatever the user had beside the chat.
    openView: (id) => navigator().go({ view: id }),
    openViewInDock: (id) => {
      selectChat();
      useContextDockStore.getState().openDockView(id);
    },
    selectDockView: (id) => useContextDockStore.getState().selectDockView(id),
    closeDockView: (id) => useContextDockStore.getState().closeDockView(id),
    collapseDock: () => useContextDockStore.getState().collapseDock(),
    showDock: (defaultViewId) => {
      selectChat();
      useContextDockStore.getState().showDock(defaultViewId);
    },
    /** A stale id is a no-op: it is not the surface on screen. */
    closeView: (id) => {
      if (navigator().get().view === id) selectChat();
    },
    activeViewId: () => navigator().get().view,
    dock: () => {
      const state = useContextDockStore.getState();
      return {
        open: state.dockOpen,
        viewIds: state.dockViewIds,
        activeViewId: state.activeDockViewId,
      };
    },
    setSettingsPane: (pane) => navigator().go({ settings: pane }),
    settingsPaneTarget: () => navigator().get().settings,
    setActiveFile: (path) => useContextDockStore.getState().setActiveFile(path),
    openFile: (path, line) => {
      useContextDockStore.getState().setFileViewer(path, line);
      useContextDockStore.getState().openDockView("file");
      selectChat();
    },
    selectedToolId: () => useContextDockStore.getState().selectedToolId,
    setSelectedTool: (id) => useContextDockStore.getState().setSelectedToolId(id),
    locateTool: (id) => {
      selectChat();
      useContextDockStore.getState().revealTool(id);
      if (!focusConversationTool(id) && typeof requestAnimationFrame === "function") {
        requestAnimationFrame(() => focusConversationTool(id));
      }
    },
    activateSessionScope: (sessionId) =>
      useContextDockStore.getState().activateSessionScope(sessionId),
    forgetSessionScopes: (openSessionIds) =>
      useContextDockStore.getState().forgetSessionScopes(openSessionIds),
  });
}

function focusConversationTool(itemId: string): boolean {
  const anchor = document.getElementById(itemId);
  if (!anchor) return false;
  anchor.scrollIntoView?.({ block: "center" });
  anchor.querySelector<HTMLElement>("button")?.focus({ preventScroll: true });
  return true;
}
