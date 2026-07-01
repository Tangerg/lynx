import { useUiStore } from "@/state/uiStore";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";
import { configureWorkspaceNavigationPort } from "../application/ports/navigationState";

export function installWorkspaceNavigationPort(): void {
  configureWorkspaceNavigationPort({
    useActiveViewId: () => useWorkspaceNavigationStore((state) => state.activeMainView),
    useSplitViewId: () => useWorkspaceNavigationStore((state) => state.splitViewId),
    useActiveFile: () => useWorkspaceNavigationStore((state) => state.activeFile),
    useFileViewer: () => useWorkspaceNavigationStore((state) => state.fileViewer),
    useSettingsPaneTarget: () => useWorkspaceNavigationStore((state) => state.settingsPane),
    useExpandedToolIds: () => useWorkspaceNavigationStore((state) => state.expandedToolIds),
    useSelectTool: () => useWorkspaceNavigationStore((state) => state.setSelectedToolId),
    useToggleTool: () => useWorkspaceNavigationStore((state) => state.toggleExpandedTool),
    useSidebarRail: () => {
      const preferRail = useUiStore((state) => state.sidebarRail);
      const splitOpen = useWorkspaceNavigationStore((state) => state.splitViewId !== null);
      return preferRail || splitOpen;
    },
    selectChat: () => useWorkspaceNavigationStore.getState().selectChat(),
    openView: (tab) => useWorkspaceNavigationStore.getState().openMainView(tab),
    openViewBeside: (tab) => useWorkspaceNavigationStore.getState().openMainViewBeside(tab),
    closeView: (id) => useWorkspaceNavigationStore.getState().closeMainView(id),
    activeViewId: () => useWorkspaceNavigationStore.getState().activeMainView,
    closeSplit: () => useWorkspaceNavigationStore.getState().closeSplit(),
    promoteSplitToView: () => useWorkspaceNavigationStore.getState().promoteSplitToTab(),
    setSettingsPane: (pane) => useWorkspaceNavigationStore.getState().setSettingsPane(pane),
    settingsPaneTarget: () => useWorkspaceNavigationStore.getState().settingsPane,
    setActiveFile: (path) => useWorkspaceNavigationStore.getState().setActiveFile(path),
    openFile: (path, line) => useWorkspaceNavigationStore.getState().openFileViewer(path, line),
    selectedToolId: () => useWorkspaceNavigationStore.getState().selectedToolId,
    setSelectedTool: (id) => useWorkspaceNavigationStore.getState().setSelectedToolId(id),
    clearSessionState: () => useWorkspaceNavigationStore.getState().clearSessionScopedState(),
  });
}
