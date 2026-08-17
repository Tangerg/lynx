import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore, WorkspaceFileFocus } from "@/state/contextDockStore";
import { navigator } from "@/lib/navigation";
import {
  closeActiveWorkspaceDockView,
  closeActiveWorkspaceView,
  collapseWorkspaceDock,
  locateWorkspaceTool,
  openWorkspaceView,
  openWorkspaceViewInDock,
  reconcileWorkspaceToolSelection,
  selectWorkspaceDockView,
  showWorkspaceDock,
  toggleWorkspaceDock,
  activateWorkspaceSessionScope,
} from "./navigation";

function reset() {
  document.body.replaceChildren();
  navigator().go({ view: "v2", settings: null });
  useContextDockStore.setState({
    activeSessionScopeId: null,
    sessionScopes: new Map(),
    dockViewIds: [],
    lastViewId: null,
    fileFocus: WorkspaceFileFocus.empty(),
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
      dockViewIds: ["explorer", "diff"],
      lastViewId: "explorer",
    });
  });

  it("only selects views that already belong to the dock", () => {
    openWorkspaceViewInDock("explorer");
    selectWorkspaceDockView("diff");
    expect(navigator().get().dock).toBe("explorer");
  });

  it("a full view leaves the dock workspace alone", () => {
    openWorkspaceViewInDock("explorer");
    collapseWorkspaceDock();
    openWorkspaceView("v3");

    expect(navigator().get().view).toBe("v3");
    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["explorer"],
      lastViewId: "explorer",
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
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });
  });

  it("showing an empty dock starts in Explorer", () => {
    showWorkspaceDock();

    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["explorer"],
      lastViewId: "explorer",
    });
  });

  it("toggles the visible dock without discarding its session tabs", () => {
    openWorkspaceViewInDock("diff");

    toggleWorkspaceDock();
    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);

    toggleWorkspaceDock();
    expect(navigator().get().dock).toBe("diff");
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("does not reopen a collapsed dock when the same session scope is rebound", () => {
    navigator().go({ session: "s1", dock: null });
    useContextDockStore.setState({
      activeSessionScopeId: "s1",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });

    activateWorkspaceSessionScope("s1");

    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState().dockViewIds).toEqual(["diff"]);
  });

  it("distinguishes a real move from the initialized sessionless scope", () => {
    navigator().go({ session: "s1", dock: "diff" });
    useContextDockStore.setState({
      activeSessionScopeId: "",
      dockViewIds: ["diff"],
      lastViewId: "diff",
    });

    activateWorkspaceSessionScope("s1");

    expect(navigator().get().dock).toBeNull();
    expect(useContextDockStore.getState()).toMatchObject({
      activeSessionScopeId: "s1",
      dockViewIds: [],
      lastViewId: null,
    });
  });

  it("closes the active dock tab before the session-level command can run", () => {
    openWorkspaceViewInDock("explorer");
    openWorkspaceViewInDock("diff");

    expect(closeActiveWorkspaceDockView()).toBe(true);
    expect(useContextDockStore.getState()).toMatchObject({
      dockViewIds: ["explorer"],
      lastViewId: "explorer",
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

  it("reconciles a stale tool selection to the latest surviving item", () => {
    useContextDockStore.setState({ selectedToolId: "tool-gone" });

    reconcileWorkspaceToolSelection(["tool-old", "tool-latest"]);

    expect(useContextDockStore.getState().selectedToolId).toBe("tool-latest");
  });

  it("clears the selected tool when an authoritative snapshot has none", () => {
    useContextDockStore.setState({ selectedToolId: "tool-gone" });

    reconcileWorkspaceToolSelection([]);

    expect(useContextDockStore.getState().selectedToolId).toBe("");
  });
});
