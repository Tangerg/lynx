import {
  workspaceNavigation,
  type WorkspaceColumnWidth,
  type WorkspaceDockSnapshot,
  type WorkspaceFileViewer,
} from "./ports/navigationState";

const DEFAULT_DOCK_VIEW_ID = "explorer";

export function useActiveWorkspaceViewId(): string | null {
  return workspaceNavigation().useActiveViewId();
}

export function useWorkspaceDock(): WorkspaceDockSnapshot {
  return workspaceNavigation().useDock();
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

/** The drawer preference consumed by the window shell's single stable control. */
export function useSidebarDrawer(): { collapsed: boolean; toggle: () => void } {
  return workspaceNavigation().useSidebarDrawer();
}

export function useSidebarWidth(): WorkspaceColumnWidth {
  return workspaceNavigation().useSidebarWidth();
}

export function useDockWidth(): WorkspaceColumnWidth {
  return workspaceNavigation().useDockWidth();
}

export function selectWorkspaceChat(): void {
  workspaceNavigation().selectChat();
}

/** Give a view the whole content card. Reserved for surfaces that have nothing
 *  to say beside a conversation, such as settings. */
export function openWorkspaceView(id: string): void {
  workspaceNavigation().openView(id);
}

/** Open a view in the dock, beside the conversation — the default placement for
 *  anything opened *from* the conversation or the palette. */
export function openWorkspaceViewInDock(id: string): void {
  workspaceNavigation().openViewInDock(id);
}

export function selectWorkspaceDockView(id: string): void {
  workspaceNavigation().selectDockView(id);
}

export function closeWorkspaceDockView(id: string): void {
  workspaceNavigation().closeDockView(id);
}

export function closeActiveWorkspaceDockView(): boolean {
  const activeViewId = workspaceNavigation().dock().activeViewId;
  if (!activeViewId) return false;
  workspaceNavigation().closeDockView(activeViewId);
  return true;
}

export function collapseWorkspaceDock(): void {
  workspaceNavigation().collapseDock();
}

export function showWorkspaceDock(): void {
  workspaceNavigation().showDock(DEFAULT_DOCK_VIEW_ID);
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

export function openWorkspaceSettingsPane(pane: string): void {
  workspaceNavigation().setSettingsPane(pane);
  workspaceNavigation().openView("settings");
}

export function getWorkspaceSettingsPaneTarget(): string | null {
  return workspaceNavigation().settingsPaneTarget();
}

export function clearWorkspaceSettingsPaneTarget(): void {
  workspaceNavigation().setSettingsPane(null);
}

export function openWorkspaceDiffForFile(path: string): void {
  workspaceNavigation().setActiveFile(path);
  workspaceNavigation().openViewInDock("diff");
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

export function locateWorkspaceTool(id: string): void {
  workspaceNavigation().locateTool(id);
}

export function activateWorkspaceSessionScope(sessionId: string): void {
  workspaceNavigation().activateSessionScope(sessionId);
}

export function forgetWorkspaceSessionScopes(openSessionIds: string[]): void {
  workspaceNavigation().forgetSessionScopes(openSessionIds);
}
