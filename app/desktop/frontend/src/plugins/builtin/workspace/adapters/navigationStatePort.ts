import { useUiStore } from "@/state/uiStore";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";

export function installWorkspaceNavigationPort(): () => void {
  return configureWorkspaceNavigationPort({
    useActiveViewId: () => useWorkspaceSurfaceStore((state) => state.activeMainView),
    useDockViewId: () => useContextDockStore((state) => state.dockViewId),
    useActiveFile: () => useContextDockStore((state) => state.activeFile),
    useFileViewer: () => useContextDockStore((state) => state.fileViewer),
    useSettingsPaneTarget: () => useWorkspaceSurfaceStore((state) => state.settingsPane),
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
    useDockWidth: () => ({
      width: useUiStore((state) => state.dockWidth),
      setWidth: useUiStore((state) => state.setDockWidth),
    }),
    selectChat: () => useWorkspaceSurfaceStore.getState().selectChat(),
    // Taking the whole card leaves the dock's own selection alone: closing the
    // full view brings back whatever the user had beside the chat.
    openView: (id) => useWorkspaceSurfaceStore.getState().openMainView(id),
    openViewInDock: (id) => {
      useWorkspaceSurfaceStore.getState().selectChat();
      useContextDockStore.getState().openDockView(id);
    },
    closeView: (id) => {
      useWorkspaceSurfaceStore.getState().closeMainView(id);
      useContextDockStore.getState().closeDockViewIf(id);
    },
    activeViewId: () => useWorkspaceSurfaceStore.getState().activeMainView,
    dockViewId: () => useContextDockStore.getState().dockViewId,
    lastDockViewId: () => useContextDockStore.getState().lastDockViewId,
    closeDockView: () => useContextDockStore.getState().closeDockView(),
    promoteDockViewToFull: () => {
      const dockViewId = useContextDockStore.getState().dockViewId;
      if (!dockViewId) return;
      useWorkspaceSurfaceStore.getState().openMainView(dockViewId);
      useContextDockStore.getState().closeDockView();
    },
    setSettingsPane: (pane) => useWorkspaceSurfaceStore.getState().setSettingsPane(pane),
    settingsPaneTarget: () => useWorkspaceSurfaceStore.getState().settingsPane,
    setActiveFile: (path) => useContextDockStore.getState().setActiveFile(path),
    openFile: (path, line) => {
      useContextDockStore.getState().setFileViewer(path, line);
      useContextDockStore.getState().openDockView("file");
      useWorkspaceSurfaceStore.getState().selectChat();
    },
    selectedToolId: () => useContextDockStore.getState().selectedToolId,
    setSelectedTool: (id) => useContextDockStore.getState().setSelectedToolId(id),
    activateSessionScope: (sessionId) =>
      useContextDockStore.getState().activateSessionScope(sessionId),
    forgetSessionScopes: (openSessionIds) =>
      useContextDockStore.getState().forgetSessionScopes(openSessionIds),
  });
}
