import { create } from "zustand";
import { disposeOnHmr } from "@/lib/hmr";
import { useAgentSessionStore } from "./agentSessionStore";

interface MainViewTab {
  id: string;
  title: string;
  icon?: string;
}

interface WorkspaceFileViewer {
  path: string;
  line: number;
}

interface WorkspaceNavigationState {
  mainViewTabs: MainViewTab[];
  activeMainView: string | null;
  settingsPane: string | null;
  splitViewId: string | null;
  activeFile: string;
  fileViewer: WorkspaceFileViewer | null;
  selectedToolId: string;
  expandedToolIds: Set<string>;
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
  clearSessionScopedState: () => void;
}

function sessionScopedWorkspacePatch() {
  return {
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
    splitViewId: null,
  };
}

export const useWorkspaceNavigationStore = create<
  WorkspaceNavigationState & WorkspaceNavigationActions
>((set, get) => ({
  mainViewTabs: [],
  activeMainView: null,
  settingsPane: null,
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
  clearSessionScopedState: () => set(sessionScopedWorkspacePatch()),
}));

const unsubAgentSessionSelection = useAgentSessionStore.subscribe((state, prev) => {
  const workspace = useWorkspaceNavigationStore.getState();
  if (state.selectionEpoch !== prev.selectionEpoch) workspace.selectChat();
  if (state.activeSessionId !== prev.activeSessionId) workspace.clearSessionScopedState();
});
disposeOnHmr(unsubAgentSessionSelection);
