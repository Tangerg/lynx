import { create } from "zustand";

export interface WorkspaceFileViewer {
  path: string;
  line: number;
}

interface ContextDockSessionScope {
  /** Visibility is independent from the open view set. Collapsing the dock is
   *  therefore lossless: tabs and their mounted view state remain intact. */
  dockOpen: boolean;
  dockViewIds: string[];
  activeDockViewId: string | null;
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
  selectDockView: (id: string) => void;
  closeDockView: (id: string) => void;
  collapseDock: () => void;
  showDock: (defaultViewId: string) => void;
  setActiveFile: (path: string) => void;
  setFileViewer: (path: string, line?: number) => void;
  setSelectedToolId: (id: string) => void;
  revealTool: (id: string) => void;
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
    dockOpen: false,
    dockViewIds: [],
    activeDockViewId: null,
  };
}

function cloneSessionScope(scope: ContextDockSessionScope): ContextDockSessionScope {
  return {
    activeFile: scope.activeFile,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    selectedToolId: scope.selectedToolId,
    expandedToolIds: new Set(scope.expandedToolIds),
    dockOpen: scope.dockOpen,
    dockViewIds: [...scope.dockViewIds],
    activeDockViewId: scope.activeDockViewId,
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
  dockOpen: false,
  dockViewIds: [],
  activeDockViewId: null,
  activeFile: "",
  fileViewer: null,
  selectedToolId: "",
  expandedToolIds: new Set<string>(),

  openDockView: (id) =>
    set((state) => ({
      dockOpen: true,
      dockViewIds: state.dockViewIds.includes(id) ? state.dockViewIds : [...state.dockViewIds, id],
      activeDockViewId: id,
    })),
  selectDockView: (id) =>
    set((state) =>
      state.dockViewIds.includes(id) ? { dockOpen: true, activeDockViewId: id } : {},
    ),
  closeDockView: (id) =>
    set((state) => {
      const index = state.dockViewIds.indexOf(id);
      if (index < 0) return {};

      const dockViewIds = state.dockViewIds.filter((viewId) => viewId !== id);
      if (state.activeDockViewId !== id) return { dockViewIds };

      const activeDockViewId = dockViewIds[index] ?? dockViewIds[index - 1] ?? null;
      return {
        dockOpen: activeDockViewId !== null,
        dockViewIds,
        activeDockViewId,
      };
    }),
  collapseDock: () => set({ dockOpen: false }),
  showDock: (defaultViewId) =>
    set((state) => {
      if (state.dockViewIds.length === 0) {
        return {
          dockOpen: true,
          dockViewIds: [defaultViewId],
          activeDockViewId: defaultViewId,
        };
      }
      return {
        dockOpen: true,
        activeDockViewId: state.dockViewIds.includes(state.activeDockViewId ?? "")
          ? state.activeDockViewId
          : (state.dockViewIds[0] ?? null),
      };
    }),
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
