import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

/** A resizable column of the content card: its width and the setter a drag
 *  commits to. Both edges of the card (drawer, dock) are sized this way. */
export interface WorkspaceColumnWidth {
  width: number;
  setWidth: (width: number) => void;
}

export interface WorkspaceNavigationPort {
  useActiveViewId(): string | null;
  useDockViewId(): string | null;
  useActiveFile(): string;
  useFileViewer(): WorkspaceFileViewer | null;
  useSettingsPaneTarget(): string | null;
  useExpandedToolIds(): Set<string>;
  useSelectTool(): (id: string) => void;
  useToggleTool(): (id: string) => void;
  useSidebarDrawer(): { collapsed: boolean; toggle: () => void };
  useSidebarWidth(): WorkspaceColumnWidth;
  useDockWidth(): WorkspaceColumnWidth;
  selectChat(): void;
  openView(id: string): void;
  openViewInDock(id: string): void;
  closeView(id: string): void;
  activeViewId(): string | null;
  dockViewId(): string | null;
  lastDockViewId(): string | null;
  closeDockView(): void;
  promoteDockViewToFull(): void;
  setSettingsPane(pane: string | null): void;
  settingsPaneTarget(): string | null;
  setActiveFile(path: string): void;
  openFile(path: string, line?: number): void;
  selectedToolId(): string;
  setSelectedTool(id: string): void;
  activateSessionScope(sessionId: string): void;
  forgetSessionScopes(openSessionIds: string[]): void;
}

const port = createSingletonPort<WorkspaceNavigationPort>(
  "Workspace navigation port is not configured",
);

export const configureWorkspaceNavigationPort = port.configure;
export const workspaceNavigation = port.get;
