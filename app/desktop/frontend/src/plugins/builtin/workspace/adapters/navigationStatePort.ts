import { useUiStore } from "@/state/uiStore";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { WORKSPACE_VIEW, lookupExtensionByKey } from "@/plugins/sdk";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";
import type { WorkspaceViewTab } from "../application/ports/navigationState";
import type { WorkspaceSurfaceTab } from "@/state/workspaceSurfaceStore";

function withWorkspaceViewMetadata(tab: WorkspaceViewTab): WorkspaceSurfaceTab {
  const view = lookupExtensionByKey(WORKSPACE_VIEW, tab.id);
  return {
    id: tab.id,
    title: tab.title ?? view?.title ?? tab.id,
    icon: tab.icon ?? view?.icon,
  };
}

export function installWorkspaceNavigationPort(): () => void {
  return configureWorkspaceNavigationPort({
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
      useWorkspaceSurfaceStore.getState().openMainView(withWorkspaceViewMetadata(tab));
      useContextDockStore.getState().closeSplit();
    },
    openViewBeside: (tab) => {
      useWorkspaceSurfaceStore.getState().ensureMainViewTab(withWorkspaceViewMetadata(tab));
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
      useWorkspaceSurfaceStore.getState().openMainView(withWorkspaceViewMetadata({ id: "file" }));
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
