import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore } from "./contextDockStore";

function reset() {
  useContextDockStore.setState({
    activeSessionScopeId: "",
    sessionScopes: new Map(),
    splitViewId: null,
    activeFile: "",
    fileViewer: null,
    selectedToolId: "",
    expandedToolIds: new Set<string>(),
  });
}

describe("context dock session scopes", () => {
  beforeEach(reset);

  function seedDock() {
    useContextDockStore.setState({
      activeFile: "src/a.ts",
      selectedToolId: "tool-1",
      expandedToolIds: new Set(["tool-1"]),
      splitViewId: "diff",
    });
  }

  function expectBlankScope() {
    const s = useContextDockStore.getState();
    expect(s.activeFile).toBe("");
    expect(s.selectedToolId).toBe("");
    expect(s.expandedToolIds.size).toBe(0);
    expect(s.splitViewId).toBeNull();
  }

  function expectSeededScope() {
    const s = useContextDockStore.getState();
    expect(s.activeFile).toBe("src/a.ts");
    expect(s.selectedToolId).toBe("tool-1");
    expect(s.expandedToolIds.has("tool-1")).toBe(true);
    expect(s.splitViewId).toBe("diff");
  }

  // oxlint-disable-next-line vitest/expect-expect -- helper functions contain the assertions.
  it("activateSessionScope saves and restores each session's dock state", () => {
    useContextDockStore.getState().activateSessionScope("s1");
    seedDock();

    useContextDockStore.getState().activateSessionScope("s2");
    expectBlankScope();

    useContextDockStore.setState({
      activeFile: "src/b.ts",
      selectedToolId: "tool-2",
      expandedToolIds: new Set(["tool-2"]),
      splitViewId: "terminal",
    });

    useContextDockStore.getState().activateSessionScope("s1");
    expectSeededScope();

    useContextDockStore.getState().activateSessionScope("s2");
    const s = useContextDockStore.getState();
    expect(s.activeFile).toBe("src/b.ts");
    expect(s.selectedToolId).toBe("tool-2");
    expect(s.expandedToolIds.has("tool-2")).toBe(true);
    expect(s.splitViewId).toBe("terminal");
  });

  // oxlint-disable-next-line vitest/expect-expect -- helper functions contain the assertions.
  it("forgetSessionScopes drops dock state for sessions no longer open", () => {
    useContextDockStore.getState().activateSessionScope("s1");
    seedDock();
    useContextDockStore.getState().activateSessionScope("s2");
    useContextDockStore.getState().forgetSessionScopes(["s2"]);

    useContextDockStore.getState().activateSessionScope("s1");

    expectBlankScope();
  });
});
