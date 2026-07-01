export interface WorkspaceViewTab {
  id: string;
  title: string;
  icon?: string;
}

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

export interface WorkspaceNavigationPort {
  useActiveViewId(): string | null;
  useSplitViewId(): string | null;
  useActiveFile(): string;
  useFileViewer(): WorkspaceFileViewer | null;
  useSettingsPaneTarget(): string | null;
  useExpandedToolIds(): Set<string>;
  useSelectTool(): (id: string) => void;
  useToggleTool(): (id: string) => void;
  useSidebarRail(): boolean;
  selectChat(): void;
  openView(tab: WorkspaceViewTab): void;
  openViewBeside(tab: WorkspaceViewTab): void;
  closeView(id: string): void;
  activeViewId(): string | null;
  closeSplit(): void;
  promoteSplitToView(): void;
  setSettingsPane(pane: string | null): void;
  settingsPaneTarget(): string | null;
  setActiveFile(path: string): void;
  openFile(path: string, line?: number): void;
  selectedToolId(): string;
  setSelectedTool(id: string): void;
  clearSessionState(): void;
}

let port: WorkspaceNavigationPort | null = null;

export function configureWorkspaceNavigationPort(next: WorkspaceNavigationPort): void {
  port = next;
}

export function workspaceNavigation(): WorkspaceNavigationPort {
  if (!port) throw new Error("Workspace navigation port is not configured");
  return port;
}
