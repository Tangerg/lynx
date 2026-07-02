import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { closeContextDockView, openContextDockView } from "./contextDock";

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

  it("closes only the dock view", () => {
    openContextDockView({ id: "tools", title: "workspace.view.title.tools", icon: "tool" });
    closeContextDockView();

    expect(useContextDockStore.getState().splitViewId).toBeNull();
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });
});
