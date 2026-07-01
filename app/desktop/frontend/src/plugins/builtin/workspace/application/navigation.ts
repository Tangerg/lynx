import { useSessionStore } from "@/state/sessionStore";

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
  return useSessionStore((state) => state.activeMainView);
}

export function useSplitWorkspaceViewId(): string | null {
  return useSessionStore((state) => state.splitViewId);
}

export function useActiveWorkspaceFile(): string {
  return useSessionStore((state) => state.activeFile);
}

export function useWorkspaceFileViewer(): WorkspaceFileViewer | null {
  return useSessionStore((state) => state.fileViewer);
}

export function useExpandedWorkspaceToolIds(): Set<string> {
  return useSessionStore((state) => state.expandedToolIds);
}

export function useSelectWorkspaceTool(): (id: string) => void {
  return useSessionStore((state) => state.setSelectedToolId);
}

export function useToggleWorkspaceTool(): (id: string) => void {
  return useSessionStore((state) => state.toggleExpandedTool);
}

export function selectWorkspaceChat(): void {
  useSessionStore.getState().selectChat();
}

export function openWorkspaceViewBeside(tab: WorkspaceViewTab): void {
  useSessionStore.getState().openMainViewBeside(tab);
}

export function closeWorkspaceView(id: string): void {
  useSessionStore.getState().closeMainView(id);
}

export function closeWorkspaceSplit(): void {
  useSessionStore.getState().closeSplit();
}

export function promoteWorkspaceSplitToView(): void {
  useSessionStore.getState().promoteSplitToTab();
}

export function openWorkspaceSettingsPane(pane: string, title: string): void {
  const workspace = useSessionStore.getState();
  workspace.setSettingsPane(pane);
  workspace.openMainView({ id: "settings", title, icon: "settings" });
}

export function openWorkspaceDiffForFile(path: string): void {
  const workspace = useSessionStore.getState();
  workspace.setActiveFile(path);
  workspace.openMainView({ id: "diff", title: "workspace.view.title.diff", icon: "diff" });
}

export function focusWorkspaceFile(path: string): void {
  useSessionStore.getState().setActiveFile(path);
}

export function openWorkspaceFile(path: string, line?: number): void {
  useSessionStore.getState().openFileViewer(path, line);
}

export function selectWorkspaceTool(id: string): void {
  useSessionStore.getState().setSelectedToolId(id);
}

export function selectInitialWorkspaceTool(id: string): void {
  const workspace = useSessionStore.getState();
  if (!workspace.selectedToolId) workspace.setSelectedToolId(id);
}
