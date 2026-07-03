import { beforeEach, describe, expect, it } from "vitest";
import { agentTextInput } from "@/plugins/builtin/agent/domain/input";
import { useAgentSessionStore } from "./agentSessionStore";

// Snapshot of the store's initial open-session state so each test starts
// from a known place. We restore via setState (not resetForTest — no
// such action exists on this store) and let the persist middleware
// rewrite localStorage on its own.
const INITIAL = {
  activeSessionId: "s1",
  openSessionIds: ["s1", "s2", "s3"],
};

function reset() {
  useAgentSessionStore.setState({
    activeSessionId: INITIAL.activeSessionId,
    openSessionIds: [...INITIAL.openSessionIds],
  });
}

describe("selectSession after empty state", () => {
  beforeEach(reset);

  it("adds the first session to openSessionIds from an empty state", () => {
    useAgentSessionStore.setState({ openSessionIds: [], activeSessionId: "" });
    useAgentSessionStore.getState().selectSession("s1");
    const s = useAgentSessionStore.getState();
    expect(s.openSessionIds).toEqual(["s1"]);
    expect(s.activeSessionId).toBe("s1");
  });

  it("appends a second session without dropping the first", () => {
    useAgentSessionStore.setState({ openSessionIds: [], activeSessionId: "" });
    useAgentSessionStore.getState().selectSession("s1");
    useAgentSessionStore.getState().selectSession("s2");
    const s = useAgentSessionStore.getState();
    expect(s.openSessionIds).toEqual(["s1", "s2"]);
    expect(s.activeSessionId).toBe("s2");
  });

  it("closeSession on the last session → selectSession chain leaves correct state", () => {
    // Alternate path: close the very last session, then
    // open new ones from the sidebar.
    useAgentSessionStore.setState({ openSessionIds: ["s1"], activeSessionId: "s1" });
    useAgentSessionStore.getState().closeSession("s1");
    // Closing the only session falls back to "" (welcome screen) — next[0] is
    // undefined, so activeSessionId is cleared, never left pointing at the
    // closed session.
    expect(useAgentSessionStore.getState().openSessionIds).toEqual([]);
    expect(useAgentSessionStore.getState().activeSessionId).toBe("");

    useAgentSessionStore.getState().selectSession("s2");
    expect(useAgentSessionStore.getState().openSessionIds).toEqual(["s2"]);

    useAgentSessionStore.getState().selectSession("s3");
    expect(useAgentSessionStore.getState().openSessionIds).toEqual(["s2", "s3"]);
  });
});

describe("agentSessionStore draft lifecycle", () => {
  beforeEach(() => {
    useAgentSessionStore.setState({
      activeSessionId: "",
      openSessionIds: [],
      draftSessionIds: new Set<string>(),
      pendingMessages: {},
    });
  });

  it("markDraft hides a session; graduateDraft reveals it", () => {
    const s = useAgentSessionStore.getState();
    s.markDraft("d1");
    expect(useAgentSessionStore.getState().draftSessionIds.has("d1")).toBe(true);
    s.graduateDraft("d1");
    expect(useAgentSessionStore.getState().draftSessionIds.has("d1")).toBe(false);
  });

  it("graduateDraft on a non-draft is a no-op", () => {
    useAgentSessionStore.getState().graduateDraft("nope");
    expect(useAgentSessionStore.getState().draftSessionIds.size).toBe(0);
  });

  it("takePendingMessage returns then clears the queued first message", () => {
    const s = useAgentSessionStore.getState();
    s.setPendingMessage("d1", { input: agentTextInput("hello"), runOptions: {} });
    expect(useAgentSessionStore.getState().takePendingMessage("d1")).toEqual({
      input: agentTextInput("hello"),
      runOptions: {},
    });
    // consumed — second take is undefined
    expect(useAgentSessionStore.getState().takePendingMessage("d1")).toBeUndefined();
  });

  it("takePendingMessage is undefined when nothing queued", () => {
    expect(useAgentSessionStore.getState().takePendingMessage("x")).toBeUndefined();
  });
});

describe("reconcileSessions reconciles persisted open sessions against backend truth", () => {
  beforeEach(reset);

  it("drops every open session + clears active when the backend has no sessions (db reset)", () => {
    // s1/s2/s3 were persisted across a launch, but `make fresh` wiped the db —
    // the runtime now has none, so a send would hit session_not_found.
    useAgentSessionStore.setState({ draftSessionIds: new Set() });
    useAgentSessionStore.getState().reconcileSessions([]);
    const s = useAgentSessionStore.getState();
    expect(s.openSessionIds).toEqual([]);
    expect(s.activeSessionId).toBe(""); // → welcome screen, never a dead session
  });

  it("keeps live sessions and a not-yet-graduated draft, prunes the rest", () => {
    // s1 still live on the backend; s2 deleted; s3 is a fresh draft (created up
    // front, absent from sessions.list until its first message graduates it).
    useAgentSessionStore.setState({ draftSessionIds: new Set(["s3"]) });
    useAgentSessionStore.getState().reconcileSessions(["s1"]);
    const s = useAgentSessionStore.getState();
    expect(s.openSessionIds).toEqual(["s1", "s3"]); // s2 pruned, draft kept
    expect(s.activeSessionId).toBe("s1"); // active still alive
  });

  it("re-targets the active session to a survivor when it was dropped", () => {
    useAgentSessionStore.setState({ draftSessionIds: new Set() });
    useAgentSessionStore.getState().reconcileSessions(["s2", "s3"]); // active s1 is gone
    const s = useAgentSessionStore.getState();
    expect(s.openSessionIds).toEqual(["s2", "s3"]);
    expect(s.activeSessionId).toBe("s3"); // falls back to the last survivor
  });

  it("is a no-op (same openSessionIds reference) when every persisted session is still live", () => {
    useAgentSessionStore.setState({ draftSessionIds: new Set() });
    const before = useAgentSessionStore.getState().openSessionIds;
    useAgentSessionStore.getState().reconcileSessions(["s1", "s2", "s3"]);
    const s = useAgentSessionStore.getState();
    expect(s.openSessionIds).toBe(before); // early return — no set(), reference unchanged
    expect(s.activeSessionId).toBe("s1");
  });
});
