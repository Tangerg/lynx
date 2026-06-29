import { beforeEach, describe, expect, it } from "vitest";
import { useSessionStore } from "./sessionStore";

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
    // A leftover split would leak the collapsed-rail state into the next
    // describe block; baseline it so each test starts from a truly known place.
    splitViewId: null,
  });
}

describe("selectTab returns the main pane to chat", () => {
  beforeEach(reset);

  it("clears activeMainView — selecting a session while a view is promoted must not no-op", () => {
    expect(useSessionStore.getState().activeMainView).toBe("v2");
    useSessionStore.getState().selectTab("s2");
    const s = useSessionStore.getState();
    expect(s.activeSessionId).toBe("s2");
    expect(s.activeMainView).toBeNull();
  });
});

describe("split (beside) view", () => {
  beforeEach(reset);

  it("promoteSplitToTab moves the split view to a full tab and clears the split", () => {
    // v2 is already a known tab; open it beside chat, then promote.
    useSessionStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    expect(useSessionStore.getState().splitViewId).toBe("v2");

    useSessionStore.getState().promoteSplitToTab();
    const s = useSessionStore.getState();
    expect(s.splitViewId).toBeNull();
    expect(s.activeMainView).toBe("v2");
    // Promotion reuses the existing tab — no duplicate appended.
    expect(s.mainViewTabs.map((t) => t.id)).toEqual(["v1", "v2", "v3"]);
  });

  it("promoteSplitToTab is a no-op when no split is open", () => {
    useSessionStore.setState({ splitViewId: null, activeMainView: "v2" });
    useSessionStore.getState().promoteSplitToTab();
    const s = useSessionStore.getState();
    expect(s.splitViewId).toBeNull();
    expect(s.activeMainView).toBe("v2");
  });

  it("openMainViewBeside and openMainView are mutually exclusive", () => {
    useSessionStore.getState().openMainViewBeside({ id: "v1", title: "View 1" });
    expect(useSessionStore.getState().activeMainView).toBeNull();
    useSessionStore.getState().openMainView({ id: "v1", title: "View 1" });
    expect(useSessionStore.getState().splitViewId).toBeNull();
  });

  it("switching to a DIFFERENT session closes the split (rail must not stay hidden on a session that never opened one)", () => {
    useSessionStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    expect(useSessionStore.getState().splitViewId).toBe("v2");
    useSessionStore.getState().selectTab("s2");
    expect(useSessionStore.getState().splitViewId).toBeNull();
  });

  it("re-selecting the SAME session keeps the split open", () => {
    // active = s1 (reset baseline)
    useSessionStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    useSessionStore.getState().selectTab("s1");
    expect(useSessionStore.getState().splitViewId).toBe("v2");
  });

  it("closing the active tab drops its split", () => {
    useSessionStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    useSessionStore.getState().closeTab("s1"); // s1 is active
    expect(useSessionStore.getState().splitViewId).toBeNull();
  });

  it("closing a background tab leaves the active session's split intact", () => {
    useSessionStore.getState().openMainViewBeside({ id: "v2", title: "View 2" });
    useSessionStore.getState().closeTab("s3"); // s3 is not active (s1 is)
    expect(useSessionStore.getState().splitViewId).toBe("v2");
  });
});

describe("session-scoped view state resets on every session switch", () => {
  beforeEach(reset);

  // Pretend the user was inspecting a tool in s1: an open file, a selected +
  // expanded tool, and a beside-split. All four are session-scoped.
  function seedInspector(activeSessionId = "s1") {
    useSessionStore.setState({
      activeSessionId,
      activeFile: "src/a.ts",
      selectedToolId: "tool-1",
      expandedToolIds: new Set(["tool-1"]),
      splitViewId: "diff",
    });
  }

  function expectCleared() {
    const s = useSessionStore.getState();
    expect(s.activeFile).toBe("");
    expect(s.selectedToolId).toBe("");
    expect(s.expandedToolIds.size).toBe(0);
    expect(s.splitViewId).toBeNull();
  }

  function expectPreserved() {
    const s = useSessionStore.getState();
    expect(s.activeFile).toBe("src/a.ts");
    expect(s.selectedToolId).toBe("tool-1");
    expect(s.expandedToolIds.has("tool-1")).toBe(true);
    expect(s.splitViewId).toBe("diff");
  }

  // oxlint-disable-next-line vitest/expect-expect — expectCleared() is a helper that contains assertions.
  it("selectTab to a different session clears all four fields", () => {
    seedInspector();
    useSessionStore.getState().selectTab("s2");
    expectCleared();
  });

  // oxlint-disable-next-line vitest/expect-expect — expectPreserved() is a helper that contains assertions.
  it("re-selecting the SAME session preserves them", () => {
    seedInspector();
    useSessionStore.getState().selectTab("s1");
    expectPreserved();
  });

  it("closing the active tab clears the state of the session left", () => {
    seedInspector(); // active s1, tabs s1,s2,s3
    useSessionStore.getState().closeTab("s1");
    expect(useSessionStore.getState().activeSessionId).toBe("s2");
    expectCleared();
  });

  it("closing a BACKGROUND tab keeps the active session's state", () => {
    seedInspector(); // active s1
    useSessionStore.getState().closeTab("s3");
    expect(useSessionStore.getState().activeSessionId).toBe("s1");
    expectPreserved();
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

  it("closeTab on the last tab → selectTab chain leaves correct state", () => {
    // Alternate path: close the very last tab by clicking its X, then
    // open new ones from the sidebar.
    useSessionStore.setState({ tabIds: ["s1"], activeSessionId: "s1" });
    useSessionStore.getState().closeTab("s1");
    // Closing the only tab falls back to "" (welcome screen) — next[0] is
    // undefined, so activeSessionId is cleared, never left pointing at the
    // closed session.
    expect(useSessionStore.getState().tabIds).toEqual([]);
    expect(useSessionStore.getState().activeSessionId).toBe("");

    useSessionStore.getState().selectTab("s2");
    expect(useSessionStore.getState().tabIds).toEqual(["s2"]);

    useSessionStore.getState().selectTab("s3");
    expect(useSessionStore.getState().tabIds).toEqual(["s2", "s3"]);
  });
});

describe("sessionStore draft lifecycle", () => {
  beforeEach(() => {
    useSessionStore.setState({
      activeSessionId: "",
      tabIds: [],
      draftSessionIds: new Set<string>(),
      pendingMessages: {},
    });
  });

  it("markDraft hides a session; graduateDraft reveals it", () => {
    const s = useSessionStore.getState();
    s.markDraft("d1");
    expect(useSessionStore.getState().draftSessionIds.has("d1")).toBe(true);
    s.graduateDraft("d1");
    expect(useSessionStore.getState().draftSessionIds.has("d1")).toBe(false);
  });

  it("graduateDraft on a non-draft is a no-op", () => {
    useSessionStore.getState().graduateDraft("nope");
    expect(useSessionStore.getState().draftSessionIds.size).toBe(0);
  });

  it("takePendingMessage returns then clears the queued first message", () => {
    const s = useSessionStore.getState();
    s.setPendingMessage("d1", [{ type: "text", text: "hello" }]);
    expect(useSessionStore.getState().takePendingMessage("d1")).toEqual([
      { type: "text", text: "hello" },
    ]);
    // consumed — second take is undefined
    expect(useSessionStore.getState().takePendingMessage("d1")).toBeUndefined();
  });

  it("takePendingMessage is undefined when nothing queued", () => {
    expect(useSessionStore.getState().takePendingMessage("x")).toBeUndefined();
  });
});
