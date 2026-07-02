import { beforeEach, describe, expect, it } from "vitest";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";
import { closeContextDockView, openContextDockView } from "./contextDock";

describe("context dock navigation", () => {
  beforeEach(() => {
    useWorkspaceNavigationStore.setState({
      splitViewId: null,
      activeMainView: null,
      mainViewTabs: [],
    });
  });

  it("opens workspace material beside the agent narrative", () => {
    openContextDockView({ id: "search", title: "workspace.view.title.search", icon: "search" });

    const state = useWorkspaceNavigationStore.getState();
    expect(state.splitViewId).toBe("search");
    expect(state.activeMainView).toBeNull();
  });

  it("closes only the dock view", () => {
    openContextDockView({ id: "tools", title: "workspace.view.title.tools", icon: "tool" });
    closeContextDockView();

    const state = useWorkspaceNavigationStore.getState();
    expect(state.splitViewId).toBeNull();
    expect(state.activeMainView).toBeNull();
  });
});
