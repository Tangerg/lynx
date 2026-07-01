import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useAgentSessionStore } from "@/plugins/builtin/agent/adapters/agentSessionStore";
import { loadPlugin, unloadPlugin } from "@/plugins/sdk";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";
import sessionNavigation from ".";

const sessions = ["s1", "s2", "s3"];
const views = [
  { id: "v1", title: "View 1" },
  { id: "v2", title: "View 2" },
  { id: "v3", title: "View 3" },
];

function resetAgentSessions() {
  useAgentSessionStore.setState({
    activeSessionId: "s1",
    selectionEpoch: 0,
    tabIds: [...sessions],
    draftSessionIds: new Set<string>(),
    pendingMessages: {},
  });
}

function resetWorkspace() {
  useWorkspaceNavigationStore.setState({
    mainViewTabs: views.map((view) => ({ ...view })),
    activeMainView: "v2",
    settingsPane: null,
    splitViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
}

function seedInspector() {
  useWorkspaceNavigationStore.setState({
    activeFile: "src/a.ts",
    selectedToolId: "tool-1",
    expandedToolIds: new Set(["tool-1"]),
    splitViewId: "diff",
  });
}

function expectSessionScopedStateCleared() {
  const state = useWorkspaceNavigationStore.getState();
  expect(state.activeFile).toBe("");
  expect(state.selectedToolId).toBe("");
  expect(state.expandedToolIds.size).toBe(0);
  expect(state.splitViewId).toBeNull();
}

function expectSessionScopedStatePreserved() {
  const state = useWorkspaceNavigationStore.getState();
  expect(state.activeFile).toBe("src/a.ts");
  expect(state.selectedToolId).toBe("tool-1");
  expect(state.expandedToolIds.has("tool-1")).toBe(true);
  expect(state.splitViewId).toBe("diff");
}

describe("workspace session navigation", () => {
  beforeEach(async () => {
    resetAgentSessions();
    resetWorkspace();
    await loadPlugin(sessionNavigation);
  });

  afterEach(() => {
    unloadPlugin(sessionNavigation.name);
  });

  it("selecting a different session returns the main pane to chat", () => {
    expect(useWorkspaceNavigationStore.getState().activeMainView).toBe("v2");

    useAgentSessionStore.getState().selectTab("s2");

    expect(useAgentSessionStore.getState().activeSessionId).toBe("s2");
    expect(useWorkspaceNavigationStore.getState().activeMainView).toBeNull();
  });

  // oxlint-disable-next-line vitest/expect-expect -- expectSessionScopedStateCleared contains the assertions.
  it("selecting a different session clears session-scoped workspace state", () => {
    seedInspector();

    useAgentSessionStore.getState().selectTab("s2");

    expectSessionScopedStateCleared();
  });

  // oxlint-disable-next-line vitest/expect-expect -- expectSessionScopedStatePreserved contains the assertions.
  it("re-selecting the same session preserves session-scoped workspace state", () => {
    seedInspector();

    useAgentSessionStore.getState().selectTab("s1");

    expectSessionScopedStatePreserved();
  });

  // oxlint-disable-next-line vitest/expect-expect -- expectSessionScopedStateCleared contains the assertions.
  it("closing the active session clears the workspace state of the session left", () => {
    seedInspector();

    useAgentSessionStore.getState().closeTab("s1");

    expectSessionScopedStateCleared();
  });

  // oxlint-disable-next-line vitest/expect-expect -- expectSessionScopedStatePreserved contains the assertions.
  it("closing a background session keeps the active session's workspace state", () => {
    seedInspector();

    useAgentSessionStore.getState().closeTab("s3");

    expectSessionScopedStatePreserved();
  });
});
