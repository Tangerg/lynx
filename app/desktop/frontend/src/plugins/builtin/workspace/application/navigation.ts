import {
  workspaceNavigation,
  type WorkspaceFileViewer,
  type WorkspaceViewTab,
} from "./ports/navigationState";

export function useActiveWorkspaceViewId(): string | null {
  return workspaceNavigation().useActiveViewId();
}

export function useSplitWorkspaceViewId(): string | null {
  return workspaceNavigation().useSplitViewId();
}

export function useActiveWorkspaceFile(): string {
  return workspaceNavigation().useActiveFile();
}

export function useWorkspaceFileViewer(): WorkspaceFileViewer | null {
  return workspaceNavigation().useFileViewer();
}

export function useWorkspaceSettingsPaneTarget(): string | null {
  return workspaceNavigation().useSettingsPaneTarget();
}

export function useExpandedWorkspaceToolIds(): Set<string> {
  return workspaceNavigation().useExpandedToolIds();
}

export function useSelectWorkspaceTool(): (id: string) => void {
  return workspaceNavigation().useSelectTool();
}

export function useToggleWorkspaceTool(): (id: string) => void {
  return workspaceNavigation().useToggleTool();
}

export function selectWorkspaceChat(): void {
  workspaceNavigation().selectChat();
}

export function openWorkspaceView(tab: WorkspaceViewTab): void {
  workspaceNavigation().openView(tab);
}

export function openWorkspaceViewBeside(tab: WorkspaceViewTab): void {
  workspaceNavigation().openViewBeside(tab);
}

export function closeWorkspaceView(id: string): void {
  workspaceNavigation().closeView(id);
}

export function closeActiveWorkspaceView(): boolean {
  const activeViewId = workspaceNavigation().activeViewId();
  if (!activeViewId) return false;
  workspaceNavigation().closeView(activeViewId);
  return true;
}

export function closeWorkspaceSplit(): void {
  workspaceNavigation().closeSplit();
}

export function promoteWorkspaceSplitToView(): void {
  workspaceNavigation().promoteSplitToView();
}

export function openWorkspaceSettingsPane(pane: string, title: string): void {
  workspaceNavigation().setSettingsPane(pane);
  workspaceNavigation().openView({ id: "settings", title, icon: "settings" });
}

export function getWorkspaceSettingsPaneTarget(): string | null {
  return workspaceNavigation().settingsPaneTarget();
}

export function clearWorkspaceSettingsPaneTarget(): void {
  workspaceNavigation().setSettingsPane(null);
}

export function openWorkspaceDiffForFile(path: string): void {
  workspaceNavigation().setActiveFile(path);
  workspaceNavigation().openView({ id: "diff", title: "workspace.view.title.diff", icon: "diff" });
}

export function focusWorkspaceFile(path: string): void {
  workspaceNavigation().setActiveFile(path);
}

export function openWorkspaceFile(path: string, line?: number): void {
  workspaceNavigation().openFile(path, line);
}

export function selectWorkspaceTool(id: string): void {
  workspaceNavigation().setSelectedTool(id);
}

export function selectInitialWorkspaceTool(id: string): void {
  if (!workspaceNavigation().selectedToolId()) workspaceNavigation().setSelectedTool(id);
}

export function clearWorkspaceSessionState(): void {
  workspaceNavigation().clearSessionState();
}
