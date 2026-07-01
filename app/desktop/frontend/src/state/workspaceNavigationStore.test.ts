import { beforeEach, describe, expect, it } from "vitest";
import { useAgentSessionStore } from "./agentSessionStore";
import { useWorkspaceNavigationStore } from "./workspaceNavigationStore";

const sessions = ["s1", "s2", "s3"];
const views = [
  { id: "v1", title: "View 1" },
  { id: "v2", title: "View 2" },
  { id: "v3", title: "View 3" },
];

function reset() {
  useAgentSessionStore.setState({
    activeSessionId: "s1",
    selectionEpoch: 0,
    tabIds: [...sessions],
  });
  useWorkspaceNavigationStore.setState({
    mainViewTabs: views.map((view) => ({ ...view })),
    activeMainView: "v2",
    splitViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
}

describe("workspace navigation focus", () => {
  beforeEach(reset);

  it("selecting a session returns the main pane to chat", () => {
    expect(useWorkspaceNavigationStore.getState().activeMainView).toBe("v2");
    useAgentSessionStore.getState().selectTab("s2");

    expect(useAgentSessionStore.getState().activeSessionId).toBe("s2");
    expect(useWorkspaceNavigationStore.getState().activeMainView).toBeNull();
  });
});

describe("workspace split view", () => {
  beforeEach(reset);

  it("promoteSplitToTab moves the split view to a full tab and clears the split", () => {
    useWorkspaceNavigationStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBe("v2");

    useWorkspaceNavigationStore.getState().promoteSplitToTab();
    const s = useWorkspaceNavigationStore.getState();
    expect(s.splitViewId).toBeNull();
    expect(s.activeMainView).toBe("v2");
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v1", "v2", "v3"]);
  });

  it("promoteSplitToTab is a no-op when no split is open", () => {
    useWorkspaceNavigationStore.setState({ splitViewId: null, activeMainView: "v2" });
    useWorkspaceNavigationStore.getState().promoteSplitToTab();
    const s = useWorkspaceNavigationStore.getState();
    expect(s.splitViewId).toBeNull();
    expect(s.activeMainView).toBe("v2");
  });

  it("openMainViewBeside and openMainView are mutually exclusive", () => {
    useWorkspaceNavigationStore.getState().openMainViewBeside({ id: "v1", title: "View 1" });
    expect(useWorkspaceNavigationStore.getState().activeMainView).toBeNull();
    useWorkspaceNavigationStore.getState().openMainView({ id: "v1", title: "View 1" });
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBeNull();
  });

  it("switching to a different session closes the split", () => {
    useWorkspaceNavigationStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBe("v2");
    useAgentSessionStore.getState().selectTab("s2");
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBeNull();
  });

  it("re-selecting the same session keeps the split open", () => {
    useWorkspaceNavigationStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    useAgentSessionStore.getState().selectTab("s1");
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBe("v2");
  });

  it("closing the active tab drops its split", () => {
    useWorkspaceNavigationStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    useAgentSessionStore.getState().closeTab("s1");
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBeNull();
  });

  it("closing a background tab leaves the active session's split intact", () => {
    useWorkspaceNavigationStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    useAgentSessionStore.getState().closeTab("s3");
    expect(useWorkspaceNavigationStore.getState().splitViewId).toBe("v2");
  });
});

describe("session-scoped workspace state", () => {
  beforeEach(reset);

  function seedInspector(activeSessionId = "s1") {
    useAgentSessionStore.setState({ activeSessionId });
    useWorkspaceNavigationStore.setState({
      activeFile: "src/a.ts",
      selectedToolId: "tool-1",
      expandedToolIds: new Set(["tool-1"]),
      splitViewId: "diff",
    });
  }

  function expectCleared() {
    const s = useWorkspaceNavigationStore.getState();
    expect(s.activeFile).toBe("");
    expect(s.selectedToolId).toBe("");
    expect(s.expandedToolIds.size).toBe(0);
    expect(s.splitViewId).toBeNull();
  }

  function expectPreserved() {
    const s = useWorkspaceNavigationStore.getState();
    expect(s.activeFile).toBe("src/a.ts");
    expect(s.selectedToolId).toBe("tool-1");
    expect(s.expandedToolIds.has("tool-1")).toBe(true);
    expect(s.splitViewId).toBe("diff");
  }

  // oxlint-disable-next-line vitest/expect-expect -- expectCleared contains the assertions.
  it("selectTab to a different session clears session-scoped workspace state", () => {
    seedInspector();
    useAgentSessionStore.getState().selectTab("s2");
    expectCleared();
  });

  // oxlint-disable-next-line vitest/expect-expect -- expectPreserved contains the assertions.
  it("re-selecting the same session preserves session-scoped workspace state", () => {
    seedInspector();
    useAgentSessionStore.getState().selectTab("s1");
    expectPreserved();
  });

  it("closing the active tab clears the workspace state of the session left", () => {
    seedInspector();
    useAgentSessionStore.getState().closeTab("s1");
    expect(useAgentSessionStore.getState().activeSessionId).toBe("s2");
    expectCleared();
  });

  it("closing a background tab keeps the active session's workspace state", () => {
    seedInspector();
    useAgentSessionStore.getState().closeTab("s3");
    expect(useAgentSessionStore.getState().activeSessionId).toBe("s1");
    expectPreserved();
  });
});
