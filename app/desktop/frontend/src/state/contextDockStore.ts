import { create } from "zustand";

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

interface ContextDockSessionScope {
  splitViewId: string | null;
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
  openSplit: (id: string) => void;
  closeSplit: () => void;
  closeSplitIf: (id: string) => void;
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
    splitViewId: null,
  };
}

function cloneSessionScope(scope: ContextDockSessionScope): ContextDockSessionScope {
  return {
    activeFile: scope.activeFile,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    selectedToolId: scope.selectedToolId,
    expandedToolIds: new Set(scope.expandedToolIds),
    splitViewId: scope.splitViewId,
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
  splitViewId: null,
  activeFile: "",
  fileViewer: null,
  selectedToolId: "",
  expandedToolIds: new Set<string>(),

  openSplit: (id) => set({ splitViewId: id }),
  closeSplit: () => set({ splitViewId: null }),
  closeSplitIf: (id) => {
    if (get().splitViewId === id) set({ splitViewId: null });
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
