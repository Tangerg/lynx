import { beforeEach, describe, expect, it } from "vitest";
import { useWorkspaceSurfaceStore } from "./workspaceSurfaceStore";

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
}

describe("workspace surface state", () => {
  beforeEach(reset);

  it("openMainView activates a tab without duplicating existing tabs", () => {
    useWorkspaceSurfaceStore.getState().openMainView({ id: "v2", title: "View 2" });
    const state = useWorkspaceSurfaceStore.getState();

    expect(state.activeMainView).toBe("v2");
    expect(state.mainViewTabs.map((t) => t.id)).toEqual(["v1", "v2", "v3"]);
  });

  it("closeMainView falls back to the last remaining tab", () => {
    useWorkspaceSurfaceStore.getState().closeMainView("v2");
    const state = useWorkspaceSurfaceStore.getState();

    expect(state.activeMainView).toBe("v3");
    expect(state.mainViewTabs.map((t) => t.id)).toEqual(["v1", "v3"]);
  });

  it("selectChat clears the active full-surface view", () => {
    useWorkspaceSurfaceStore.getState().selectChat();

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });
});
