import { beforeEach, describe, expect, it } from "vitest";
import { useWorkspaceSurfaceStore } from "./workspaceSurfaceStore";

function reset() {
  useWorkspaceSurfaceStore.setState({ activeMainView: "v2", settingsPane: null });
}

describe("workspace surface state", () => {
  beforeEach(reset);

  it("openMainView puts a view on the whole card", () => {
    useWorkspaceSurfaceStore.getState().openMainView("v3");

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v3");
  });

  it("closeMainView returns to the chat", () => {
    useWorkspaceSurfaceStore.getState().closeMainView("v2");

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("closeMainView ignores a view that is not on screen", () => {
    useWorkspaceSurfaceStore.getState().closeMainView("v1");

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBe("v2");
  });

  it("selectChat clears the active full-surface view", () => {
    useWorkspaceSurfaceStore.getState().selectChat();

    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });
});
