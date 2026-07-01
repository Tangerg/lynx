import { beforeEach, describe, expect, it } from "vitest";
import { useWorkspaceNavigationStore } from "./workspaceNavigationStore";

const views = [
  { id: "v1", title: "View 1" },
  { id: "v2", title: "View 2" },
  { id: "v3", title: "View 3" },
];

function reset() {
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
});

describe("session-scoped workspace state", () => {
  beforeEach(reset);

  function seedInspector() {
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

  // oxlint-disable-next-line vitest/expect-expect -- expectCleared contains the assertions.
  it("clearSessionScopedState clears inspector and split state", () => {
    seedInspector();
    useWorkspaceNavigationStore.getState().clearSessionScopedState();
    expectCleared();
  });
});
