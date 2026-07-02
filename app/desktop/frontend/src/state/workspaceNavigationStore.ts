import { create } from "zustand";

interface MainViewTab {
  id: string;
  title: string;
  icon?: string;
}

interface WorkspaceFileViewer {
  path: string;
  line: number;
}

interface WorkspaceSessionScope {
  splitViewId: string | null;
  activeFile: string;
  fileViewer: WorkspaceFileViewer | null;
  selectedToolId: string;
  expandedToolIds: Set<string>;
}

interface WorkspaceNavigationState extends WorkspaceSessionScope {
  mainViewTabs: MainViewTab[];
  activeMainView: string | null;
  settingsPane: string | null;
  activeSessionScopeId: string;
  sessionScopes: Map<string, WorkspaceSessionScope>;
}

interface WorkspaceNavigationActions {
  setSettingsPane: (pane: string | null) => void;
  openMainView: (tab: MainViewTab) => void;
  closeMainView: (id: string) => void;
  selectChat: () => void;
  openMainViewBeside: (tab: MainViewTab) => void;
  closeSplit: () => void;
  promoteSplitToTab: () => void;
  setActiveFile: (path: string) => void;
  openFileViewer: (path: string, line?: number) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  activateSessionScope: (sessionId: string) => void;
  forgetSessionScopes: (openSessionIds: string[]) => void;
}

function emptySessionScope(): WorkspaceSessionScope {
  return {
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
    splitViewId: null,
  };
}

function cloneSessionScope(scope: WorkspaceSessionScope): WorkspaceSessionScope {
  return {
    activeFile: scope.activeFile,
    fileViewer: scope.fileViewer ? { ...scope.fileViewer } : null,
    selectedToolId: scope.selectedToolId,
    expandedToolIds: new Set(scope.expandedToolIds),
    splitViewId: scope.splitViewId,
  };
}

function currentSessionScope(state: WorkspaceNavigationState): WorkspaceSessionScope {
  return cloneSessionScope(state);
}

function saveCurrentSessionScope(state: WorkspaceNavigationState) {
  const scopes = new Map(state.sessionScopes);
  if (state.activeSessionScopeId)
    scopes.set(state.activeSessionScopeId, currentSessionScope(state));
  return scopes;
}

export const useWorkspaceNavigationStore = create<
  WorkspaceNavigationState & WorkspaceNavigationActions
>((set, get) => ({
  mainViewTabs: [],
  activeMainView: null,
  settingsPane: null,
  activeSessionScopeId: "",
  sessionScopes: new Map<string, WorkspaceSessionScope>(),
  splitViewId: null,
  activeFile: "",
  fileViewer: null,
  selectedToolId: "",
  expandedToolIds: new Set<string>(),

  setSettingsPane: (pane) => set({ settingsPane: pane }),
  openMainView: (tab) => {
    const cur = get().mainViewTabs;
    const exists = cur.some((t) => t.id === tab.id);
    set({
      mainViewTabs: exists ? cur : [...cur, tab],
      activeMainView: tab.id,
      splitViewId: null,
    });
  },
  openMainViewBeside: (tab) => {
    const cur = get().mainViewTabs;
    const exists = cur.some((t) => t.id === tab.id);
    set({
      mainViewTabs: exists ? cur : [...cur, tab],
      splitViewId: tab.id,
      activeMainView: null,
    });
  },
  closeSplit: () => set({ splitViewId: null }),
  promoteSplitToTab: () => {
    const { splitViewId, mainViewTabs } = get();
    const tab = splitViewId ? mainViewTabs.find((t) => t.id === splitViewId) : undefined;
    if (tab) get().openMainView(tab);
  },
  closeMainView: (id) => {
    const cur = get().mainViewTabs;
    const next = cur.filter((t) => t.id !== id);
    const activeMainView =
      get().activeMainView === id ? (next.at(-1)?.id ?? null) : get().activeMainView;
    const splitViewId = get().splitViewId === id ? null : get().splitViewId;
    set({ mainViewTabs: next, activeMainView, splitViewId });
  },
  selectChat: () => set({ activeMainView: null }),
  setActiveFile: (path) => set({ activeFile: path }),
  openFileViewer: (path, line) => {
    get().openMainView({ id: "file", title: "workspace.view.title.file", icon: "filetext" });
    set({ fileViewer: { path, line: line ?? 0 } });
  },
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
        ...cloneSessionScope(nextScope ?? emptySessionScope()),
      };
    }),
  forgetSessionScopes: (openSessionIds) =>
    set((state) => {
      const open = new Set(openSessionIds);
      const sessionScopes = new Map<string, WorkspaceSessionScope>();
      for (const [sessionId, scope] of state.sessionScopes) {
        if (open.has(sessionId)) sessionScopes.set(sessionId, scope);
      }
      return { sessionScopes };
    }),
}));
