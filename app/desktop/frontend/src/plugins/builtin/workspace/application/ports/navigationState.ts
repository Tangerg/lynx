import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

export interface WorkspaceFileFocusSnapshot {
  readonly path: string;
  readonly revision: number;
}

/** A resizable column of the content card: its width and the setter a drag
 *  commits to. Both edges of the card (drawer, dock) are sized this way. */
export interface WorkspaceColumnWidth {
  width: number;
  setWidth: (width: number) => void;
}

export interface WorkspaceDockSnapshot {
  open: boolean;
  viewIds: string[];
  activeViewId: string | null;
}

export interface WorkspaceNavigationPort {
  useActiveViewId(): string | null;
  useDock(): WorkspaceDockSnapshot;
  useFileFocus(): WorkspaceFileFocusSnapshot;
  useFileViewer(): WorkspaceFileViewer | null;
  useSettingsPaneTarget(): string | null;
  useExpandedToolIds(): Set<string>;
  useSelectedToolId(): string;
  useSelectTool(): (id: string) => void;
  useToggleTool(): (id: string) => void;
  useSidebarDrawer(): { collapsed: boolean; toggle: () => void };
  useSidebarWidth(): WorkspaceColumnWidth;
  useDockWidth(): WorkspaceColumnWidth;
  selectChat(): void;
  openView(id: string): void;
  openViewInDock(id: string): void;
  selectDockView(id: string): void;
  closeDockView(id: string): void;
  collapseDock(): void;
  showDock(defaultViewId: string): void;
  closeView(id: string): void;
  activeViewId(): string | null;
  dock(): WorkspaceDockSnapshot;
  setSettingsPane(pane: string): void;
  focusFile(path: string): void;
  openFile(path: string, line?: number): void;
  selectedToolId(): string;
  setSelectedTool(id: string): void;
  locateTool(id: string): void;
  activateSessionScope(sessionId: string): void;
  forgetSessionScopes(openSessionIds: string[]): void;
}

const port = createSingletonPort<WorkspaceNavigationPort>(
  "Workspace navigation port is not configured",
);

export const configureWorkspaceNavigationPort = port.configure;
export const workspaceNavigation = port.get;
