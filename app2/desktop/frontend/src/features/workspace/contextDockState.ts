const dockStorageKey = "lyra.app2.context-dock.v1";
const maxRememberedSessions = 32;
export const maxCollapsedReviewFiles = 100;

export const maxOpenFiles = 8;
export const maxExpandedDirectories = 100;

export type DockPane = "workspace" | "session";
export type WorkspaceView = "files" | "review";
export type ReviewMode = "worktree" | "base";
export type DiffLayout = "unified" | "split";

export interface SessionDockState {
  workspacePath: string;
  pane: DockPane;
  workspaceView: WorkspaceView;
  openPaths: string[];
  selectedPath?: string;
  expandedDirectories: string[];
  searchDraft: string;
  searchQuery: string;
  selectedChangePath?: string;
  reviewMode: ReviewMode;
  diffLayout: DiffLayout;
  reviewNavigatorOpen: boolean;
  collapsedReviewPaths: string[];
  targetLines: Record<string, number>;
  touchedAt: number;
}

export function newDockState(workspacePath: string): SessionDockState {
  return {
    workspacePath,
    pane: "session",
    workspaceView: "files",
    openPaths: [],
    expandedDirectories: [],
    searchDraft: "",
    searchQuery: "",
    reviewMode: "worktree",
    diffLayout: "unified",
    reviewNavigatorOpen: true,
    collapsedReviewPaths: [],
    targetLines: {},
    touchedAt: Date.now(),
  };
}

export function rememberDockState(
  current: Record<string, SessionDockState>,
  sessionId: string,
  state: SessionDockState,
) {
  return mostRecent({ ...current, [sessionId]: state });
}

export function readDockState(): Record<string, SessionDockState> {
  try {
    const raw = window.localStorage.getItem(dockStorageKey);
    if (raw === null) return {};
    const value: unknown = JSON.parse(raw);
    if (!isRecord(value)) return {};
    const states: Record<string, SessionDockState> = {};
    for (const [sessionId, candidate] of Object.entries(value)) {
      const state = parseDockState(candidate);
      if (state !== undefined) states[sessionId] = state;
    }
    return mostRecent(states);
  } catch {
    return {};
  }
}

export function writeDockState(states: Record<string, SessionDockState>) {
  try {
    window.localStorage.setItem(dockStorageKey, JSON.stringify(states));
  } catch {
    // Presentation state may be dropped when storage is unavailable.
  }
}

function parseDockState(value: unknown): SessionDockState | undefined {
  if (!isRecord(value) || typeof value.workspacePath !== "string") {
    return undefined;
  }
  const openPaths = stringArray(value.openPaths).slice(-maxOpenFiles);
  const selectedPath =
    typeof value.selectedPath === "string" &&
    openPaths.includes(value.selectedPath)
      ? value.selectedPath
      : undefined;
  const targetLines: Record<string, number> = {};
  if (isRecord(value.targetLines)) {
    for (const path of openPaths) {
      const line = value.targetLines[path];
      if (typeof line === "number" && Number.isInteger(line) && line > 0) {
        targetLines[path] = line;
      }
    }
  }
  return {
    workspacePath: value.workspacePath,
    pane: value.pane === "workspace" ? "workspace" : "session",
    workspaceView: value.workspaceView === "review" ? "review" : "files",
    openPaths,
    ...(selectedPath === undefined ? {} : { selectedPath }),
    expandedDirectories: stringArray(value.expandedDirectories).slice(
      -maxExpandedDirectories,
    ),
    searchDraft: typeof value.searchDraft === "string" ? value.searchDraft : "",
    searchQuery: typeof value.searchQuery === "string" ? value.searchQuery : "",
    ...(typeof value.selectedChangePath === "string"
      ? { selectedChangePath: value.selectedChangePath }
      : {}),
    reviewMode: value.reviewMode === "base" ? "base" : "worktree",
    diffLayout: value.diffLayout === "split" ? "split" : "unified",
    reviewNavigatorOpen: value.reviewNavigatorOpen !== false,
    collapsedReviewPaths: stringArray(value.collapsedReviewPaths).slice(
      -maxCollapsedReviewFiles,
    ),
    targetLines,
    touchedAt: typeof value.touchedAt === "number" ? value.touchedAt : 0,
  };
}

function mostRecent(states: Record<string, SessionDockState>) {
  return Object.fromEntries(
    Object.entries(states)
      .toSorted((left, right) => right[1].touchedAt - left[1].touchedAt)
      .slice(0, maxRememberedSessions),
  );
}

function stringArray(value: unknown) {
  return Array.isArray(value)
    ? value.filter((entry): entry is string => typeof entry === "string")
    : [];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
