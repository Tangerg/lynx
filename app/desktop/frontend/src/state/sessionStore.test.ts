import { beforeEach, describe, expect, it } from "vitest";
import { headerTabCloseActionsFor, useSessionStore } from "./sessionStore";

// Snapshot of the store's initial chat-tab state so each test starts
// from a known place. We restore via setState (not resetForTest — no
// such action exists on this store) and let the persist middleware
// rewrite localStorage on its own.
const INITIAL = {
  activeSessionId: "s1",
  tabIds: ["s1", "s2", "s3"],
  mainViewTabs: [
    { id: "v1", title: "View 1" },
    { id: "v2", title: "View 2" },
    { id: "v3", title: "View 3" },
  ],
  activeMainView: "v2" as string | null,
};

function reset() {
  useSessionStore.setState({
    activeSessionId: INITIAL.activeSessionId,
    tabIds: [...INITIAL.tabIds],
    mainViewTabs: INITIAL.mainViewTabs.map((t) => ({ ...t })),
    activeMainView: INITIAL.activeMainView,
  });
}

describe("sessionStore multi-tab close (chat tabs)", () => {
  beforeEach(reset);

  it("closeOtherTabs keeps only the target tab and focuses it", () => {
    useSessionStore.getState().closeOtherTabs("s2");
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s2"]);
    expect(s.activeSessionId).toBe("s2");
  });

  it("closeOtherTabs is a no-op when the id is not open", () => {
    useSessionStore.getState().closeOtherTabs("missing");
    expect(useSessionStore.getState().tabIds).toEqual(["s1", "s2", "s3"]);
  });

  it("closeTabsLeftOf drops everything before the pivot", () => {
    useSessionStore.getState().closeTabsLeftOf("s3");
    expect(useSessionStore.getState().tabIds).toEqual(["s3"]);
  });

  it("closeTabsLeftOf is a no-op when pivot is the first tab", () => {
    useSessionStore.getState().closeTabsLeftOf("s1");
    expect(useSessionStore.getState().tabIds).toEqual(["s1", "s2", "s3"]);
  });

  it("closeTabsLeftOf preserves activeSessionId when it survives", () => {
    useSessionStore.setState({ activeSessionId: "s3" });
    useSessionStore.getState().closeTabsLeftOf("s2");
    expect(useSessionStore.getState().activeSessionId).toBe("s3");
  });

  it("closeTabsLeftOf reassigns activeSessionId to the pivot when the active tab is dropped", () => {
    // active = s1, drop everything before s2 → active becomes s2
    useSessionStore.getState().closeTabsLeftOf("s2");
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s2", "s3"]);
    expect(s.activeSessionId).toBe("s2");
  });

  it("closeTabsRightOf drops everything after the pivot", () => {
    useSessionStore.getState().closeTabsRightOf("s1");
    expect(useSessionStore.getState().tabIds).toEqual(["s1"]);
  });

  it("closeTabsRightOf is a no-op when pivot is the last tab", () => {
    useSessionStore.getState().closeTabsRightOf("s3");
    expect(useSessionStore.getState().tabIds).toEqual(["s1", "s2", "s3"]);
  });

  it("closeAllTabs empties the strip", () => {
    useSessionStore.getState().closeAllTabs();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual([]);
    expect(s.activeSessionId).toBe("");
  });
});

describe("sessionStore multi-tab close (workspace-view tabs)", () => {
  beforeEach(reset);

  it("closeOtherMainViews keeps only the target view and focuses it", () => {
    useSessionStore.getState().closeOtherMainViews("v3");
    const s = useSessionStore.getState();
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v3"]);
    expect(s.activeMainView).toBe("v3");
  });

  it("closeMainViewsLeftOf drops everything before the pivot", () => {
    useSessionStore.getState().closeMainViewsLeftOf("v3");
    expect(useSessionStore.getState().mainViewTabs.map((t) => t.id)).toEqual(["v3"]);
  });

  it("closeMainViewsLeftOf is a no-op when pivot is the first view", () => {
    useSessionStore.getState().closeMainViewsLeftOf("v1");
    expect(useSessionStore.getState().mainViewTabs.map((t) => t.id)).toEqual(["v1", "v2", "v3"]);
  });

  it("closeMainViewsLeftOf reassigns activeMainView to the pivot when the active view is dropped", () => {
    // active = v2, drop everything before v3 → active becomes v3
    useSessionStore.getState().closeMainViewsLeftOf("v3");
    const s = useSessionStore.getState();
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v3"]);
    expect(s.activeMainView).toBe("v3");
  });

  it("closeMainViewsRightOf drops everything after the pivot", () => {
    useSessionStore.getState().closeMainViewsRightOf("v1");
    expect(useSessionStore.getState().mainViewTabs.map((t) => t.id)).toEqual(["v1"]);
  });

  it("closeAllMainViews empties the strip and clears focus", () => {
    useSessionStore.getState().closeAllMainViews();
    const s = useSessionStore.getState();
    expect(s.mainViewTabs).toEqual([]);
    expect(s.activeMainView).toBeNull();
  });
});

describe("selectTab after empty state", () => {
  beforeEach(reset);

  it("adds the first session to tabIds from a zero-tab state", () => {
    useSessionStore.setState({ tabIds: [], activeSessionId: "" });
    useSessionStore.getState().selectTab("s1");
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s1"]);
    expect(s.activeSessionId).toBe("s1");
  });

  it("appends a second session without dropping the first", () => {
    useSessionStore.setState({ tabIds: [], activeSessionId: "" });
    useSessionStore.getState().selectTab("s1");
    useSessionStore.getState().selectTab("s2");
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s1", "s2"]);
    expect(s.activeSessionId).toBe("s2");
  });

  it("closeAllTabs → selectTab chain leaves correct state", () => {
    // Mirror the user-reported flow: close everything, then start a
    // new session and a second new session via sidebar clicks.
    useSessionStore.getState().closeAllTabs();
    expect(useSessionStore.getState().tabIds).toEqual([]);
    expect(useSessionStore.getState().activeSessionId).toBe("");

    useSessionStore.getState().selectTab("s5");
    expect(useSessionStore.getState().tabIds).toEqual(["s5"]);

    useSessionStore.getState().selectTab("s6");
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s5", "s6"]);
    expect(s.activeSessionId).toBe("s6");
  });

  it("closeTab on the last tab → selectTab chain leaves correct state", () => {
    // Alternate path: close the very last tab by clicking its X, then
    // open new ones from the sidebar.
    useSessionStore.setState({ tabIds: ["s1"], activeSessionId: "s1" });
    useSessionStore.getState().closeTab("s1");
    // activeSessionId intentionally not cleared by closeTab when the
    // closed tab was the only one — preserves the "what was I last on"
    // hint for the next selection.
    expect(useSessionStore.getState().tabIds).toEqual([]);

    useSessionStore.getState().selectTab("s2");
    expect(useSessionStore.getState().tabIds).toEqual(["s2"]);

    useSessionStore.getState().selectTab("s3");
    expect(useSessionStore.getState().tabIds).toEqual(["s2", "s3"]);
  });
});

describe("headerTabCloseActionsFor (unified-strip semantics)", () => {
  beforeEach(reset);

  it("close All on any tab wipes BOTH lists", () => {
    headerTabCloseActionsFor("chat", "s2").onCloseAll();
    let s = useSessionStore.getState();
    expect(s.tabIds).toEqual([]);
    expect(s.mainViewTabs).toEqual([]);

    reset();
    headerTabCloseActionsFor("view", "v2").onCloseAll();
    s = useSessionStore.getState();
    expect(s.tabIds).toEqual([]);
    expect(s.mainViewTabs).toEqual([]);
  });

  it("close Others on a chat tab keeps that chat tab and drops every view tab", () => {
    headerTabCloseActionsFor("chat", "s2").onCloseOthers();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s2"]);
    expect(s.mainViewTabs).toEqual([]);
  });

  it("close Others on a view tab drops every chat tab and keeps only that view", () => {
    headerTabCloseActionsFor("view", "v2").onCloseOthers();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual([]);
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v2"]);
  });

  it("close Right on a chat tab drops trailing chat tabs AND every view tab", () => {
    headerTabCloseActionsFor("chat", "s2").onCloseRight();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s1", "s2"]);
    expect(s.mainViewTabs).toEqual([]);
  });

  it("close Right on a view tab only touches trailing view tabs (chat tabs untouched)", () => {
    headerTabCloseActionsFor("view", "v2").onCloseRight();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s1", "s2", "s3"]);
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v1", "v2"]);
  });

  it("close Left on a chat tab only touches preceding chat tabs (view tabs untouched)", () => {
    headerTabCloseActionsFor("chat", "s2").onCloseLeft();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual(["s2", "s3"]);
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v1", "v2", "v3"]);
  });

  it("close Left on a view tab drops EVERY chat tab and preceding view tabs", () => {
    headerTabCloseActionsFor("view", "v2").onCloseLeft();
    const s = useSessionStore.getState();
    expect(s.tabIds).toEqual([]);
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v2", "v3"]);
  });
});
