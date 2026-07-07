import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import {
  openWorkspaceView,
  openWorkspaceViewBeside,
  promoteWorkspaceSplitToView,
} from "./navigation";

const views = [
  { id: "v1", title: "View 1" },
  { id: "v2", title: "View 2" },
  { id: "v3", title: "View 3" },
];

function reset() {
  useWorkspaceSurfaceStore.setState({
    mainViewTabs: views.map((view) => ({ ...view })),
    activeMainView: "v2",
    settingsPane: null,
  });
  useContextDockStore.setState({
    activeSessionScopeId: "",
    sessionScopes: new Map(),
    splitViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
}

describe("workspace navigation port", () => {
  beforeEach(reset);

  it("promoteWorkspaceSplitToView moves the split view to a full tab and clears the split", () => {
    openWorkspaceViewBeside("v2");
    expect(useContextDockStore.getState().splitViewId).toBe("v2");

    promoteWorkspaceSplitToView();

    expect(useContextDockStore.getState().splitViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v2");
    expect(useWorkspaceSurfaceStore.getState().mainViewTabs.map((t) => t.id)).toEqual([
      "v1",
      "v2",
      "v3",
    ]);
  });

  it("promoteWorkspaceSplitToView is a no-op when no split is open", () => {
    useContextDockStore.setState({ splitViewId: null });

    promoteWorkspaceSplitToView();

    expect(useContextDockStore.getState().splitViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v2");
  });

  it("openWorkspaceViewBeside and openWorkspaceView are mutually exclusive", () => {
    openWorkspaceViewBeside("v1");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();

    openWorkspaceView("v1");

    expect(useContextDockStore.getState().splitViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v1");
  });
});
