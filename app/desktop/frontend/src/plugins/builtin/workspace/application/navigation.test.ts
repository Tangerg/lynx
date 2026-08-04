import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { navigator } from "@/lib/navigation";
import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  collapseWorkspaceDock,
  locateWorkspaceTool,
  openWorkspaceView,
  openWorkspaceViewInDock,
  selectWorkspaceDockView,
  showWorkspaceDock,
} from "./navigation";

function reset() {
  document.body.replaceChildren();
  navigator().go({ view: "v2", settings: null });
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

describe("workspace navigation port", () => {
  beforeEach(reset);

  it("opens dock views as stable singleton tabs beside chat", () => {
    openWorkspaceViewInDock("explorer");
    openWorkspaceViewInDock("diff");
    openWorkspaceViewInDock("explorer");

    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: true,
      dockViewIds: ["explorer", "diff"],
      activeDockViewId: "explorer",
    });
  });

  it("only selects views that already belong to the dock", () => {
    openWorkspaceViewInDock("explorer");
    selectWorkspaceDockView("diff");
    expect(useContextDockStore.getState().activeDockViewId).toBe("explorer");
  });

  it("a full view leaves the dock workspace alone", () => {
    openWorkspaceViewInDock("explorer");
    collapseWorkspaceDock();
    openWorkspaceView("v3");

    expect(navigator().get().view).toBe("v3");
    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: false,
      dockViewIds: ["explorer"],
      activeDockViewId: "explorer",
    });
  });

  it("closing a full view returns to chat without changing dock tabs", () => {
    openWorkspaceViewInDock("explorer");
    openWorkspaceView("v3");

    expect(closeActiveWorkspaceView()).toBe(true);
    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["explorer"]);
  });

  it("collapse and show are a lossless round trip", () => {
    openWorkspaceViewInDock("diff");
    collapseWorkspaceDock();
    showWorkspaceDock();

    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: true,
      dockViewIds: ["diff"],
      activeDockViewId: "diff",
    });
  });

  it("showing an empty dock starts in Explorer", () => {
    showWorkspaceDock();

    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: true,
      dockViewIds: ["explorer"],
      activeDockViewId: "explorer",
    });
  });

  it("closes the active dock tab before the session-level command can run", () => {
    openWorkspaceViewInDock("explorer");
    openWorkspaceViewInDock("diff");

    expect(closeActiveWorkspaceDockView()).toBe(true);
    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: true,
      dockViewIds: ["explorer"],
      activeDockViewId: "explorer",
    });
  });

  it("locates a parent task by selecting chat and atomically revealing its tool", () => {
    const anchor = document.createElement("div");
    anchor.id = "task-item";
    anchor.scrollIntoView = () => {};
    const button = document.createElement("button");
    anchor.append(button);
    document.body.append(anchor);

    locateWorkspaceTool("task-item");

    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().selectedToolId).toBe("task-item");
    expect(useContextDockStore.getState().expandedToolIds).toEqual(new Set(["task-item"]));
    expect(document.activeElement).toBe(button);
  });
});
