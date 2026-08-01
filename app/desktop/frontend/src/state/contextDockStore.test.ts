import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "./contextDockStore";

function reset() {
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

describe("context dock workspace", () => {
  beforeEach(reset);

  it("adds singleton views and focuses an existing view without reordering it", () => {
    const dock = useContextDockStore.getState();
    dock.openDockView("explorer");
    dock.openDockView("diff");
    dock.openDockView("explorer");

    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: true,
      dockViewIds: ["explorer", "diff"],
      activeDockViewId: "explorer",
    });
  });

  it("selects the adjacent tab after closing the active one", () => {
    const dock = useContextDockStore.getState();
    dock.openDockView("explorer");
    dock.openDockView("diff");
    dock.openDockView("terminal");

    dock.closeDockView("diff");
    expect(useContextDockStore.getState().activeDockViewId).toBe("terminal");

    dock.closeDockView("terminal");
    expect(useContextDockStore.getState().activeDockViewId).toBe("explorer");
  });

  it("keeps the view set across collapse and closes the dock with its last tab", () => {
    const dock = useContextDockStore.getState();
    dock.openDockView("diff");
    dock.collapseDock();

    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: false,
      dockViewIds: ["diff"],
      activeDockViewId: "diff",
    });

    dock.showDock("explorer");
    expect(useContextDockStore.getState().activeDockViewId).toBe("diff");

    dock.closeDockView("diff");
    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: false,
      dockViewIds: [],
      activeDockViewId: null,
    });
  });

  it("starts an empty dock on the supplied default view", () => {
    useContextDockStore.getState().showDock("explorer");

    expect(useContextDockStore.getState()).toMatchObject({
      dockOpen: true,
      dockViewIds: ["explorer"],
      activeDockViewId: "explorer",
    });
  });
});

describe("context dock session scopes", () => {
  beforeEach(reset);

  function seedDock() {
    useContextDockStore.setState({
      activeFile: "src/a.ts",
      selectedToolId: "tool-1",
      expandedToolIds: new Set(["tool-1"]),
      dockOpen: true,
      dockViewIds: ["explorer", "diff"],
      activeDockViewId: "diff",
    });
  }

  function expectBlankScope() {
    expect(useContextDockStore.getState()).toMatchObject({
      activeFile: "",
      selectedToolId: "",
      dockOpen: false,
      dockViewIds: [],
      activeDockViewId: null,
    });
    expect(useContextDockStore.getState().expandedToolIds.size).toBe(0);
  }

  function expectSeededScope() {
    expect(useContextDockStore.getState()).toMatchObject({
      activeFile: "src/a.ts",
      selectedToolId: "tool-1",
      dockOpen: true,
      dockViewIds: ["explorer", "diff"],
      activeDockViewId: "diff",
    });
    expect(useContextDockStore.getState().expandedToolIds.has("tool-1")).toBe(true);
  }

  it("saves and restores each session's full dock workspace", () => {
    useContextDockStore.getState().activateSessionScope("s1");
    seedDock();

    useContextDockStore.getState().activateSessionScope("s2");
    expectBlankScope();

    useContextDockStore.setState({
      activeFile: "src/b.ts",
      selectedToolId: "tool-2",
      expandedToolIds: new Set(["tool-2"]),
      dockOpen: false,
      dockViewIds: ["terminal"],
      activeDockViewId: "terminal",
    });

    useContextDockStore.getState().activateSessionScope("s1");
    expectSeededScope();

    useContextDockStore.getState().activateSessionScope("s2");
    expect(useContextDockStore.getState()).toMatchObject({
      activeFile: "src/b.ts",
      selectedToolId: "tool-2",
      dockOpen: false,
      dockViewIds: ["terminal"],
      activeDockViewId: "terminal",
    });
  });

  it("forgets dock workspaces for sessions no longer open", () => {
    useContextDockStore.getState().activateSessionScope("s1");
    seedDock();
    useContextDockStore.getState().activateSessionScope("s2");
    useContextDockStore.getState().forgetSessionScopes(["s2"]);

    useContextDockStore.getState().activateSessionScope("s1");
    expectBlankScope();
  });
});
