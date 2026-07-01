import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";

interface WorkspaceViewTab {
  id: string;
  title: string;
  icon?: string;
}

interface WorkspaceFileViewer {
  path: string;
  line: number;
}

export function useActiveWorkspaceViewId(): string | null {
  return useWorkspaceNavigationStore((state) => state.activeMainView);
}

export function useSplitWorkspaceViewId(): string | null {
  return useWorkspaceNavigationStore((state) => state.splitViewId);
}

export function useActiveWorkspaceFile(): string {
  return useWorkspaceNavigationStore((state) => state.activeFile);
}

export function useWorkspaceFileViewer(): WorkspaceFileViewer | null {
  return useWorkspaceNavigationStore((state) => state.fileViewer);
}

export function useWorkspaceSettingsPaneTarget(): string | null {
  return useWorkspaceNavigationStore((state) => state.settingsPane);
}

export function useExpandedWorkspaceToolIds(): Set<string> {
  return useWorkspaceNavigationStore((state) => state.expandedToolIds);
}

export function useSelectWorkspaceTool(): (id: string) => void {
  return useWorkspaceNavigationStore((state) => state.setSelectedToolId);
}

export function useToggleWorkspaceTool(): (id: string) => void {
  return useWorkspaceNavigationStore((state) => state.toggleExpandedTool);
}

export function selectWorkspaceChat(): void {
  useWorkspaceNavigationStore.getState().selectChat();
}

export function openWorkspaceView(tab: WorkspaceViewTab): void {
  useWorkspaceNavigationStore.getState().openMainView(tab);
}

export function openWorkspaceViewBeside(tab: WorkspaceViewTab): void {
  useWorkspaceNavigationStore.getState().openMainViewBeside(tab);
}

export function closeWorkspaceView(id: string): void {
  useWorkspaceNavigationStore.getState().closeMainView(id);
}

export function closeActiveWorkspaceView(): boolean {
  const workspace = useWorkspaceNavigationStore.getState();
  if (!workspace.activeMainView) return false;
  workspace.closeMainView(workspace.activeMainView);
  return true;
}

export function closeWorkspaceSplit(): void {
  useWorkspaceNavigationStore.getState().closeSplit();
}

export function promoteWorkspaceSplitToView(): void {
  useWorkspaceNavigationStore.getState().promoteSplitToTab();
}

export function openWorkspaceSettingsPane(pane: string, title: string): void {
  const workspace = useWorkspaceNavigationStore.getState();
  workspace.setSettingsPane(pane);
  workspace.openMainView({ id: "settings", title, icon: "settings" });
}

export function getWorkspaceSettingsPaneTarget(): string | null {
  return useWorkspaceNavigationStore.getState().settingsPane;
}

export function clearWorkspaceSettingsPaneTarget(): void {
  useWorkspaceNavigationStore.getState().setSettingsPane(null);
}

export function openWorkspaceDiffForFile(path: string): void {
  const workspace = useWorkspaceNavigationStore.getState();
  workspace.setActiveFile(path);
  workspace.openMainView({ id: "diff", title: "workspace.view.title.diff", icon: "diff" });
}

export function focusWorkspaceFile(path: string): void {
  useWorkspaceNavigationStore.getState().setActiveFile(path);
}

export function openWorkspaceFile(path: string, line?: number): void {
  useWorkspaceNavigationStore.getState().openFileViewer(path, line);
}

export function selectWorkspaceTool(id: string): void {
  useWorkspaceNavigationStore.getState().setSelectedToolId(id);
}

export function selectInitialWorkspaceTool(id: string): void {
  const workspace = useWorkspaceNavigationStore.getState();
  if (!workspace.selectedToolId) workspace.setSelectedToolId(id);
}
