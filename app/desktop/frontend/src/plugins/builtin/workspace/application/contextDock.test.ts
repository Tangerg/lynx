import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import {
  closeContextDockView,
  contextDockDestinationTab,
  openContextDockDestination,
  openContextDockLauncher,
  openContextDockView,
} from "./contextDock";

describe("context dock navigation", () => {
  beforeEach(() => {
    useWorkspaceSurfaceStore.setState({
      activeMainView: null,
      mainViewTabs: [],
    });
    useContextDockStore.setState({
      splitViewId: null,
    });
  });

  it("opens workspace material beside the agent narrative", () => {
    openContextDockView({ id: "search", title: "workspace.view.title.search", icon: "search" });

    expect(useContextDockStore.getState().splitViewId).toBe("search");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("opens the context launcher beside the agent narrative", () => {
    openContextDockLauncher();

    expect(useContextDockStore.getState().splitViewId).toBe("context");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("closes only the dock view", () => {
    openContextDockView({ id: "tools", title: "workspace.view.title.tools", icon: "tool" });
    closeContextDockView();

    expect(useContextDockStore.getState().splitViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("opens a contributed destination as dock material", () => {
    openContextDockDestination({
      id: "files",
      title: "workspace.view.title.files",
      icon: "filetext",
      scope: "workspace",
      placement: "context-dock",
    });

    expect(useContextDockStore.getState().splitViewId).toBe("files");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("normalizes destination tabs before opening", () => {
    expect(
      contextDockDestinationTab({
        id: "custom",
        title: "Custom",
        scope: "workspace",
        placement: "context-dock",
      }),
    ).toEqual({
      id: "custom",
      title: "Custom",
      icon: "panel-r",
    });
  });
});
