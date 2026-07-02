import { useUiStore } from "@/state/uiStore";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";

export function installWorkspaceNavigationPort(): void {
  configureWorkspaceNavigationPort({
    useActiveViewId: () => useWorkspaceSurfaceStore((state) => state.activeMainView),
    useSplitViewId: () => useContextDockStore((state) => state.splitViewId),
    useActiveFile: () => useContextDockStore((state) => state.activeFile),
    useFileViewer: () => useContextDockStore((state) => state.fileViewer),
    useSettingsPaneTarget: () => useWorkspaceSurfaceStore((state) => state.settingsPane),
    useExpandedToolIds: () => useContextDockStore((state) => state.expandedToolIds),
    useSelectTool: () => useContextDockStore((state) => state.setSelectedToolId),
    useToggleTool: () => useContextDockStore((state) => state.toggleExpandedTool),
    useSidebarRail: () => {
      const preferRail = useUiStore((state) => state.sidebarRail);
      const splitOpen = useContextDockStore((state) => state.splitViewId !== null);
      return preferRail || splitOpen;
    },
    selectChat: () => useWorkspaceSurfaceStore.getState().selectChat(),
    openView: (tab) => {
      useWorkspaceSurfaceStore.getState().openMainView(tab);
      useContextDockStore.getState().closeSplit();
    },
    openViewBeside: (tab) => {
      useWorkspaceSurfaceStore.getState().ensureMainViewTab(tab);
      useWorkspaceSurfaceStore.getState().selectChat();
      useContextDockStore.getState().openSplit(tab.id);
    },
    closeView: (id) => {
      useWorkspaceSurfaceStore.getState().closeMainView(id);
      useContextDockStore.getState().closeSplitIf(id);
    },
    activeViewId: () => useWorkspaceSurfaceStore.getState().activeMainView,
    closeSplit: () => useContextDockStore.getState().closeSplit(),
    promoteSplitToView: () => {
      const splitViewId = useContextDockStore.getState().splitViewId;
      const tab = splitViewId
        ? useWorkspaceSurfaceStore.getState().mainViewTabs.find((view) => view.id === splitViewId)
        : undefined;
      if (!tab) return;
      useWorkspaceSurfaceStore.getState().openMainView(tab);
      useContextDockStore.getState().closeSplit();
    },
    setSettingsPane: (pane) => useWorkspaceSurfaceStore.getState().setSettingsPane(pane),
    settingsPaneTarget: () => useWorkspaceSurfaceStore.getState().settingsPane,
    setActiveFile: (path) => useContextDockStore.getState().setActiveFile(path),
    openFile: (path, line) => {
      useWorkspaceSurfaceStore
        .getState()
        .openMainView({ id: "file", title: "workspace.view.title.file", icon: "filetext" });
      useContextDockStore.getState().closeSplit();
      useContextDockStore.getState().setFileViewer(path, line);
    },
    selectedToolId: () => useContextDockStore.getState().selectedToolId,
    setSelectedTool: (id) => useContextDockStore.getState().setSelectedToolId(id),
    activateSessionScope: (sessionId) =>
      useContextDockStore.getState().activateSessionScope(sessionId),
    forgetSessionScopes: (openSessionIds) =>
      useContextDockStore.getState().forgetSessionScopes(openSessionIds),
  });
}
