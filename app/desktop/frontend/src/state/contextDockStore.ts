import { create } from "zustand";

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

interface ContextDockSessionScope {
  /** The view the dock is showing. `null` IS the closed dock — there is no
   *  second "collapsed" flag to disagree with it. */
  dockViewId: string | null;
  /** What reopening restores, so the dock toggle is a round trip rather than a
   *  trip back to the launcher. Kept across a close on purpose. */
  lastDockViewId: string | null;
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
  openDockView: (id: string) => void;
  closeDockView: () => void;
  closeDockViewIf: (id: string) => void;
  setActiveFile: (path: string) => void;
  setFileViewer: (path: string, line?: number) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

function emptySessionScope(): ContextDockSessionScope {
  return {
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
    dockViewId: null,
    lastDockViewId: null,
  };
}

function cloneSessionScope(scope: ContextDockSessionScope): ContextDockSessionScope {
  return {
    activeFile: scope.activeFile,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    selectedToolId: scope.selectedToolId,
    expandedToolIds: new Set(scope.expandedToolIds),
    dockViewId: scope.dockViewId,
    lastDockViewId: scope.lastDockViewId,
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
  dockViewId: null,
  lastDockViewId: null,
  activeFile: "",
  fileViewer: null,
  selectedToolId: "",
  expandedToolIds: new Set<string>(),

  openDockView: (id) => set({ dockViewId: id, lastDockViewId: id }),
  closeDockView: () => set({ dockViewId: null }),
  closeDockViewIf: (id) => {
    if (get().dockViewId === id) set({ dockViewId: null });
  },
  setActiveFile: (path) => set({ activeFile: path }),
  setFileViewer: (path, line) => set({ fileViewer: { path, line: line ?? 0 } }),
  setSelectedToolId: (id) => set({ selectedToolId: id }),
  toggleExpandedTool: (id) => {
    const next = new Set(get().expandedToolIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    set({ expandedToolIds: next });
  },
  activateSessionScope: (sessionId) =>
    set((state) => {
      if (state.activeSessionScopeId === sessionId) return {};
      const sessionScopes = saveCurrentSessionScope(state);
      const nextScope = sessionId ? sessionScopes.get(sessionId) : undefined;
      return {
        activeSessionScopeId: sessionId,
        sessionScopes,
        ...(nextScope ? cloneSessionScope(nextScope) : emptySessionScope()),
      };
    }),
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
