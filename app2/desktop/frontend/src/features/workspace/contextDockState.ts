import type { SessionActivityView } from "../agent/agentSessionTypes";

const dockStorageKey = "lyra.app2.context-dock.v1";
const maxRememberedSessions = 32;
export const maxCollapsedReviewFiles = 100;
export const maxCodebaseQueryLength = 500;

export const maxOpenFiles = 8;
export const maxExpandedDirectories = 100;

export type DockPane = "workspace" | "session";
export type WorkspaceView = "files" | "review" | "codebase" | "skills";
export type SkillView = "available" | "proposals" | "library";
export type ReviewMode = "worktree" | "base";
export type DiffLayout = "unified" | "split";

export interface SessionDockState {
  workspacePath: string;
  pane: DockPane;
  sessionView: SessionActivityView;
  workspaceView: WorkspaceView;
  skillView: SkillView;
  openPaths: string[];
  selectedPath?: string;
  expandedDirectories: string[];
  searchDraft: string;
  searchQuery: string;
  codebaseDraft: string;
  codebaseQuery: string;
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
    sessionView: "overview",
    workspaceView: "files",
    skillView: "available",
    openPaths: [],
    expandedDirectories: [],
    searchDraft: "",
    searchQuery: "",
    codebaseDraft: "",
    codebaseQuery: "",
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
    sessionView: parseSessionView(value.sessionView),
    workspaceView:
      value.workspaceView === "review" ||
      value.workspaceView === "codebase" ||
      value.workspaceView === "skills"
        ? value.workspaceView
        : "files",
    skillView:
      value.skillView === "proposals" || value.skillView === "library"
        ? value.skillView
        : "available",
    openPaths,
    ...(selectedPath === undefined ? {} : { selectedPath }),
    expandedDirectories: stringArray(value.expandedDirectories).slice(
      -maxExpandedDirectories,
    ),
    searchDraft: typeof value.searchDraft === "string" ? value.searchDraft : "",
    searchQuery: typeof value.searchQuery === "string" ? value.searchQuery : "",
    codebaseDraft: boundedCodebaseQuery(value.codebaseDraft),
    codebaseQuery: boundedCodebaseQuery(value.codebaseQuery),
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

function parseSessionView(value: unknown): SessionActivityView {
  return value === "timeline" || value === "terminal" || value === "summary"
    ? value
    : "overview";
}

function boundedCodebaseQuery(value: unknown) {
  return typeof value === "string"
    ? value.slice(0, maxCodebaseQueryLength)
    : "";
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
