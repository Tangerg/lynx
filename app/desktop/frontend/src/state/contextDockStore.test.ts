import { beforeEach, describe, expect, it } from "vitest";
import { useContextDockStore, WorkspaceFileFocus } from "./contextDockStore";

const EMPTY = {
  activeSessionScopeId: null,
  sessionScopes: new Map(),
  dockViewIds: [],
  lastViewId: null,
  fileFocus: WorkspaceFileFocus.empty(),
  fileViewer: null,
  selectedToolId: "",
  expandedToolIds: new Set<string>(),
};

const dock = () => useContextDockStore.getState();

beforeEach(() => {
  useContextDockStore.setState({ ...EMPTY, sessionScopes: new Map() });
});

// The store holds what the dock has OPEN and what each view remembers. Which
// destination is showing is the app's location, so there is nothing here that
// can disagree with it.
describe("the open tab set", () => {
  it("holds a tab open once", () => {
    dock().openDockTab("diff");
    dock().openDockTab("diff");
    expect(dock().dockViewIds).toEqual(["diff"]);
  });

  it("answers which tab takes the place of a closed one", () => {
    dock().openDockTab("explorer");
    dock().openDockTab("diff");
    dock().openDockTab("terminal");

    expect(dock().closeDockTab("diff")).toBe("terminal");
    expect(dock().dockViewIds).toEqual(["explorer", "terminal"]);
  });

  it("falls back to the tab before it when the last one closes", () => {
    dock().openDockTab("explorer");
    dock().openDockTab("diff");

    expect(dock().closeDockTab("diff")).toBe("explorer");
  });

  it("answers null when the last tab closes, and for a tab it never had", () => {
    dock().openDockTab("diff");
    expect(dock().closeDockTab("diff")).toBeNull();
    expect(dock().closeDockTab("nope")).toBeNull();
  });
});

describe("what a re-open returns to", () => {
  it("adopts an authoritative location into both facts in one update", () => {
    let notifications = 0;
    const unsubscribe = useContextDockStore.subscribe(() => {
      notifications += 1;
    });

    dock().adoptDockLocation("diff");

    unsubscribe();
    expect(notifications).toBe(1);
    expect(dock()).toMatchObject({ dockViewIds: ["diff"], lastViewId: "diff" });
  });

  it("is the destination last shown, when it is still open", () => {
    dock().openDockTab("explorer");
    dock().openDockTab("diff");
    dock().rememberDockView("diff");

    expect(dock().dockTabToShow("terminal")).toBe("diff");
  });

  it("is the first tab when the remembered one is gone", () => {
    dock().openDockTab("explorer");
    dock().rememberDockView("diff");

    expect(dock().dockTabToShow("terminal")).toBe("explorer");
  });

  it("is the caller's default when nothing is open", () => {
    expect(dock().dockTabToShow("terminal")).toBe("terminal");
  });
});

describe("per-session scopes", () => {
  it("gives repeated file-focus intents distinct revisions", () => {
    dock().focusFile("a.ts");
    const first = dock().fileFocus;

    dock().focusFile("a.ts");

    expect(first).toMatchObject({ path: "a.ts", revision: 1 });
    expect(dock().fileFocus).toMatchObject({ path: "a.ts", revision: 2 });
    expect(dock().fileFocus).not.toBe(first);
  });

  it("keeps each session's tabs and returns the destination it remembers", () => {
    dock().activateSessionScope("s1");
    dock().openDockTab("diff");
    dock().rememberDockView("diff");
    dock().focusFile("a.ts");

    expect(dock().activateSessionScope("s2")).toBeNull();
    expect(dock().dockViewIds).toEqual([]);
    expect(dock().fileFocus).toMatchObject({ path: "", revision: 0 });

    expect(dock().activateSessionScope("s1")).toBe("diff");
    expect(dock().dockViewIds).toEqual(["diff"]);
    expect(dock().fileFocus).toMatchObject({ path: "a.ts", revision: 1 });
  });

  it("is a no-op for the session already in scope", () => {
    dock().activateSessionScope("s1");
    dock().rememberDockView("diff");

    expect(dock().activateSessionScope("s1")).toBe("diff");
    expect(dock().dockViewIds).toEqual([]);
  });

  it("forgets scopes for sessions no longer held open", () => {
    dock().activateSessionScope("s1");
    dock().openDockTab("diff");
    dock().activateSessionScope("s2");

    dock().forgetSessionScopes(["s2"]);

    expect([...dock().sessionScopes.keys()]).toEqual([]);
  });
});

describe("tool selection inside a scope", () => {
  it("reveals a tool by selecting and expanding it", () => {
    dock().revealTool("call-1");

    expect(dock().selectedToolId).toBe("call-1");
    expect(dock().expandedToolIds).toEqual(new Set(["call-1"]));
  });

  it("toggles a tool open and shut", () => {
    dock().toggleExpandedTool("call-1");
    expect(dock().expandedToolIds).toEqual(new Set(["call-1"]));

    dock().toggleExpandedTool("call-1");
    expect(dock().expandedToolIds).toEqual(new Set());
  });

  it("records a file viewer target with a default line", () => {
    dock().setFileViewer("a.ts");
    expect(dock().fileViewer).toEqual({ path: "a.ts", line: 0 });
  });
});
