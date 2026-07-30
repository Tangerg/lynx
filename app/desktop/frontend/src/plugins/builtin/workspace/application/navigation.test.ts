import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { toggleContextDock } from "./contextDock";
import {
  closeActiveWorkspaceView,
  locateWorkspaceTool,
  openWorkspaceView,
  openWorkspaceViewInDock,
  promoteWorkspaceDockViewToFull,
} from "./navigation";

function reset() {
  document.body.replaceChildren();
  useWorkspaceSurfaceStore.setState({ activeMainView: "v2", settingsPane: null });
  useContextDockStore.setState({
    activeSessionScopeId: "",
    sessionScopes: new Map(),
    dockViewId: null,
    lastDockViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
}

describe("workspace navigation port", () => {
  beforeEach(reset);

  it("promoteWorkspaceDockViewToFull hands the dock's view the whole card", () => {
    openWorkspaceViewInDock("v2");
    expect(useContextDockStore.getState().dockViewId).toBe("v2");

    promoteWorkspaceDockViewToFull();

    expect(useContextDockStore.getState().dockViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v2");
  });

  it("promoteWorkspaceDockViewToFull is a no-op when the dock is closed", () => {
    promoteWorkspaceDockViewToFull();

    expect(useContextDockStore.getState().dockViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v2");
  });

  it("a full view leaves the dock's own selection alone", () => {
    openWorkspaceViewInDock("v1");
    openWorkspaceView("v3");

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v3");
    expect(useContextDockStore.getState().dockViewId).toBe("v1");
  });

  it("closing a full view returns to the chat, never to another view", () => {
    openWorkspaceView("v1");
    openWorkspaceView("v3");

    expect(closeActiveWorkspaceView()).toBe(true);

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("toggling the dock restores the view it last held", () => {
    openWorkspaceViewInDock("v1");

    toggleContextDock();
    expect(useContextDockStore.getState().dockViewId).toBeNull();

    toggleContextDock();
    expect(useContextDockStore.getState().dockViewId).toBe("v1");
  });

  it("toggling a dock that has never been opened lands on the launcher", () => {
    toggleContextDock();

    expect(useContextDockStore.getState().dockViewId).toBe("context");
  });

  it("locates a parent task by selecting chat and atomically revealing its tool", () => {
    const anchor = document.createElement("div");
    anchor.id = "task-item";
    anchor.scrollIntoView = () => {};
    const button = document.createElement("button");
    anchor.append(button);
    document.body.append(anchor);

    locateWorkspaceTool("task-item");

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
    expect(useContextDockStore.getState().selectedToolId).toBe("task-item");
    expect(useContextDockStore.getState().expandedToolIds).toEqual(new Set(["task-item"]));
    expect(document.activeElement).toBe(button);
  });
});
