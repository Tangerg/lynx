import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentSessionLifecycleSnapshot,
  AgentSessionSelectionSnapshot,
} from "@/plugins/builtin/agent/public/session";
import { loadPlugin, unloadPlugin } from "@/plugins/sdk";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import sessionNavigation from ".";

type SelectionListener = (
  state: AgentSessionSelectionSnapshot,
  previous: AgentSessionSelectionSnapshot,
) => void;
type LifecycleListener = (state: AgentSessionLifecycleSnapshot) => void;

const agentSessionSelection = vi.hoisted(() => {
  let listener: SelectionListener | undefined;
  let lifecycleListener: LifecycleListener | undefined;
  let activeSessionId = "s1";
  let openSessionIds = ["s1", "s2"];
  return {
    emit(state: AgentSessionSelectionSnapshot, previous: AgentSessionSelectionSnapshot) {
      activeSessionId = state.activeSessionId;
      listener?.(state, previous);
    },
    emitLifecycle(state: AgentSessionLifecycleSnapshot) {
      activeSessionId = state.activeSessionId;
      openSessionIds = state.openSessionIds;
      lifecycleListener?.(state);
    },
    getActiveSessionId() {
      return activeSessionId;
    },
    getLifecycleSnapshot() {
      return { activeSessionId, openSessionIds };
    },
    reset() {
      listener = undefined;
      lifecycleListener = undefined;
      activeSessionId = "s1";
      openSessionIds = ["s1", "s2"];
    },
    subscribe(onChange: SelectionListener) {
      listener = onChange;
      return () => {
        if (listener === onChange) listener = undefined;
      };
    },
    subscribeLifecycle(onChange: LifecycleListener) {
      lifecycleListener = onChange;
      return () => {
        if (lifecycleListener === onChange) lifecycleListener = undefined;
      };
    },
  };
});

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: agentSessionSelection.getActiveSessionId,
  getAgentSessionLifecycleSnapshot: agentSessionSelection.getLifecycleSnapshot,
  subscribeAgentSessionLifecycle: agentSessionSelection.subscribeLifecycle,
  subscribeAgentSessionSelection: agentSessionSelection.subscribe,
}));

function selection(activeSessionId: string, selectionEpoch: number): AgentSessionSelectionSnapshot {
  return { activeSessionId, selectionEpoch };
}

function resetWorkspace() {
  useWorkspaceSurfaceStore.setState({
    activeMainView: "v2",
    settingsPane: null,
  });
  useContextDockStore.setState({
    activeSessionScopeId: "",
    sessionScopes: new Map(),
    dockOpen: false,
    dockViewIds: [],
    activeDockViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
}

function seedInspector() {
  useContextDockStore.setState({
    activeFile: "src/a.ts",
    selectedToolId: "tool-1",
    expandedToolIds: new Set(["tool-1"]),
    dockOpen: true,
    dockViewIds: ["explorer", "diff"],
    activeDockViewId: "diff",
  });
}

function expectSessionScopedStateBlank() {
  const state = useContextDockStore.getState();
  expect(state.activeFile).toBe("");
  expect(state.selectedToolId).toBe("");
  expect(state.expandedToolIds.size).toBe(0);
  expect(state.dockOpen).toBe(false);
  expect(state.dockViewIds).toEqual([]);
  expect(state.activeDockViewId).toBeNull();
}

function expectSessionScopedStatePreserved() {
  const state = useContextDockStore.getState();
  expect(state.activeFile).toBe("src/a.ts");
  expect(state.selectedToolId).toBe("tool-1");
  expect(state.expandedToolIds.has("tool-1")).toBe(true);
  expect(state.dockOpen).toBe(true);
  expect(state.dockViewIds).toEqual(["explorer", "diff"]);
  expect(state.activeDockViewId).toBe("diff");
}

describe("workspace session navigation", () => {
  beforeEach(async () => {
    agentSessionSelection.reset();
    resetWorkspace();
    await loadPlugin(sessionNavigation);
  });

  afterEach(() => {
    unloadPlugin(sessionNavigation.name);
  });

  it("selecting a different session returns the main pane to chat", () => {
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v2");

    agentSessionSelection.emit(selection("s2", 1), selection("s1", 0));

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("selecting a different session restores that session's workspace scope", () => {
    seedInspector();

    agentSessionSelection.emit(selection("s2", 1), selection("s1", 0));

    expectSessionScopedStateBlank();

    useContextDockStore.setState({
      activeFile: "src/b.ts",
      selectedToolId: "tool-2",
      expandedToolIds: new Set(["tool-2"]),
      dockOpen: true,
      dockViewIds: ["terminal"],
      activeDockViewId: "terminal",
    });

    agentSessionSelection.emit(selection("s1", 2), selection("s2", 1));
    expectSessionScopedStatePreserved();

    agentSessionSelection.emit(selection("s2", 3), selection("s1", 2));
    const state = useContextDockStore.getState();
    expect(state.activeFile).toBe("src/b.ts");
    expect(state.selectedToolId).toBe("tool-2");
    expect(state.expandedToolIds.has("tool-2")).toBe(true);
    expect(state.dockOpen).toBe(true);
    expect(state.dockViewIds).toEqual(["terminal"]);
    expect(state.activeDockViewId).toBe("terminal");
  });

  it("re-selecting the same session preserves session-scoped workspace state", () => {
    seedInspector();

    agentSessionSelection.emit(selection("s1", 1), selection("s1", 0));

    expectSessionScopedStatePreserved();
  });

  it("moving to a session without saved dock state starts blank", () => {
    seedInspector();

    agentSessionSelection.emit(selection("s2", 0), selection("s1", 0));

    expectSessionScopedStateBlank();
  });

  it("closing a background session keeps the active session's workspace state", () => {
    seedInspector();

    agentSessionSelection.emit(selection("s1", 0), selection("s1", 0));

    expectSessionScopedStatePreserved();
  });

  it("forgets workspace scopes for closed sessions", () => {
    seedInspector();

    agentSessionSelection.emit(selection("s2", 1), selection("s1", 0));
    agentSessionSelection.emitLifecycle({ activeSessionId: "s2", openSessionIds: ["s2"] });
    agentSessionSelection.emit(selection("s1", 2), selection("s2", 1));

    expectSessionScopedStateBlank();
  });
});
