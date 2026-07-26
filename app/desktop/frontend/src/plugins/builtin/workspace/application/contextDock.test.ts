import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "@/state/contextDockStore";
import { useWorkspaceSurfaceStore } from "@/state/workspaceSurfaceStore";
import { openContextDockDestination, openContextDockLauncher } from "./contextDock";

describe("context dock navigation", () => {
  beforeEach(() => {
    useWorkspaceSurfaceStore.setState({ activeMainView: null });
    useContextDockStore.setState({ dockViewId: null, lastDockViewId: null });
  });

  it("opens the context launcher beside the agent narrative", () => {
    openContextDockLauncher();

    expect(useContextDockStore.getState().dockViewId).toBe("context");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });

  it("opens a contributed destination as dock material", () => {
    openContextDockDestination({
      viewId: "files",
      title: "workspace.view.title.files",
      icon: "filetext",
      scope: "workspace",
    });

    expect(useContextDockStore.getState().dockViewId).toBe("files");
    expect(useWorkspaceSurfaceStore.getState().activeMainView).toBeNull();
  });
});
