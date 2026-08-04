import { create } from "zustand";

// What the dock has open, per session — not which destination is showing. That
// is the app's location (see lib/navigation), so history holds it and the dock
// no longer has a `dockOpen` flag that could disagree with the view it shows:
// the dock is open exactly when the location names a destination.
//
// `lastViewId` is the memory a re-open reads: collapsing drops the destination
// from the location, and showing the dock again should return to the tab you
// were on rather than the first one. Written from the location, never back into
// it.

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

interface ContextDockSessionScope {
  /** The open tab set. Collapsing the dock is lossless: this survives it. */
  dockViewIds: string[];
  lastViewId: string | null;
  activeFile: string;
  fileViewer: WorkspaceFileViewer | null;
  selectedToolId: string;
  expandedToolIds: Set<string>;
}

interface ContextDockState extends ContextDockSessionScope {
  activeSessionScopeId: string;
  sessionScopes: Map<string, ContextDockSessionScope>;
}

interface ContextDockActions {
  /** Hold `id` open. Which destination shows is the caller's navigation. */
  openDockTab: (id: string) => void;
  /** Drop `id`; answers which tab should take its place, or null for none. */
  closeDockTab: (id: string) => string | null;
  /** The destination a re-open should return to, given a fallback. */
  dockTabToShow: (defaultViewId: string) => string;
  rememberDockView: (id: string) => void;
  setActiveFile: (path: string) => void;
  setFileViewer: (path: string, line?: number) => void;
  setSelectedToolId: (id: string) => void;
  revealTool: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  /** Swap to `sessionId`'s scope; answers the destination it remembers. */
  activateSessionScope: (sessionId: string) => string | null;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

function emptySessionScope(): ContextDockSessionScope {
  return {
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
    dockViewIds: [],
    lastViewId: null,
  };
}

function cloneSessionScope(scope: ContextDockSessionScope): ContextDockSessionScope {
  return {
    activeFile: scope.activeFile,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    selectedToolId: scope.selectedToolId,
    expandedToolIds: new Set(scope.expandedToolIds),
    dockViewIds: [...scope.dockViewIds],
    lastViewId: scope.lastViewId,
  };
}

function saveCurrentSessionScope(state: ContextDockState) {
  const scopes = new Map(state.sessionScopes);
  if (state.activeSessionScopeId) scopes.set(state.activeSessionScopeId, cloneSessionScope(state));
  return scopes;
}

export const useContextDockStore = create<ContextDockState & ContextDockActions>((set, get) => ({
  activeSessionScopeId: "",
  sessionScopes: new Map<string, ContextDockSessionScope>(),
  dockViewIds: [],
  lastViewId: null,
  activeFile: "",
  fileViewer: null,
  selectedToolId: "",
  expandedToolIds: new Set<string>(),

  openDockTab: (id) =>
    set((state) => ({
      dockViewIds: state.dockViewIds.includes(id) ? state.dockViewIds : [...state.dockViewIds, id],
    })),
  closeDockTab: (id) => {
    const { dockViewIds } = get();
    const index = dockViewIds.indexOf(id);
    if (index < 0) return null;
    const remaining = dockViewIds.filter((viewId) => viewId !== id);
    set({ dockViewIds: remaining });
    // The tab that slid into its place, else the one before it.
    return remaining[index] ?? remaining[index - 1] ?? null;
  },
  dockTabToShow: (defaultViewId) => {
    const { dockViewIds, lastViewId } = get();
    if (dockViewIds.length === 0) return defaultViewId;
    return lastViewId !== null && dockViewIds.includes(lastViewId)
      ? lastViewId
      : (dockViewIds[0] ?? defaultViewId);
  },
  rememberDockView: (id) => set({ lastViewId: id }),
  setActiveFile: (path) => set({ activeFile: path }),
  setFileViewer: (path, line) => set({ fileViewer: { path, line: line ?? 0 } }),
  setSelectedToolId: (id) => set({ selectedToolId: id }),
  revealTool: (id) => {
    const expandedToolIds = new Set(get().expandedToolIds);
    expandedToolIds.add(id);
    set({ selectedToolId: id, expandedToolIds });
  },
  toggleExpandedTool: (id) => {
    const next = new Set(get().expandedToolIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    set({ expandedToolIds: next });
  },
  activateSessionScope: (sessionId) => {
    const state = get();
    if (state.activeSessionScopeId === sessionId) return state.lastViewId;
    const sessionScopes = saveCurrentSessionScope(state);
    const nextScope = sessionId ? sessionScopes.get(sessionId) : undefined;
    const scope = nextScope ? cloneSessionScope(nextScope) : emptySessionScope();
    set({ activeSessionScopeId: sessionId, sessionScopes, ...scope });
    return scope.lastViewId;
  },
  forgetSessionScopes: (openSessionIds) =>
    set((state) => {
      const open = new Set(openSessionIds);
      const sessionScopes = new Map<string, ContextDockSessionScope>();
      for (const [sessionId, scope] of state.sessionScopes) {
        if (open.has(sessionId)) sessionScopes.set(sessionId, scope);
      }
      return { sessionScopes };
    }),
}));
