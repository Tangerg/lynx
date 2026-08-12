import { beforeEach, describe, expect, it } from "vitest";
import { useAgentSessionStore } from "./agentSessionStore";
import { agentTextInput } from "../domain/input";

const store = () => useAgentSessionStore.getState();

beforeEach(() => {
  useAgentSessionStore.setState({
    openSessionIds: [],
    lastSessionId: "",
    draftSessionIds: new Set(),
    freshDraftSessionIds: new Set(),
    pendingMessages: {},
  });
});

// This store is memory, not location: which session is active lives in the
// URL (see lib/navigation). What is asserted here is the tab set, the cold-start
// seed, and the handoff refs — plus the pruning that keeps them from growing.
describe("the open set", () => {
  it("holds a session open once", () => {
    store().holdOpen("s1");
    store().holdOpen("s1");
    expect(store().openSessionIds).toEqual(["s1"]);
  });

  it("releases one without touching the others", () => {
    store().holdOpen("s1");
    store().holdOpen("s2");
    store().release("s1");
    expect(store().openSessionIds).toEqual(["s2"]);
  });

  it("retains only the ids boot reconciliation kept", () => {
    store().holdOpen("s1");
    store().holdOpen("s2");
    store().retainOnly(["s2"]);
    expect(store().openSessionIds).toEqual(["s2"]);
  });
});

describe("the cold-start seed", () => {
  it("remembers where the user was", () => {
    store().rememberSession("s1");
    expect(store().lastSessionId).toBe("s1");
  });
});

// The version is the discard switch: bumping it must leave no trace of the old
// payload — including in storage, or the next boot reads it again.
describe("storage written by an older version", () => {
  it("boots on defaults and restamps storage at the current version", async () => {
    localStorage.setItem(
      "lyra.agent-session",
      JSON.stringify({ state: { openSessionIds: ["stale"], lastSessionId: "stale" }, version: 1 }),
    );

    await useAgentSessionStore.persist.rehydrate();

    expect(store().openSessionIds).toEqual([]);
    expect(store().lastSessionId).toBe("");

    const stored = JSON.parse(localStorage.getItem("lyra.agent-session") ?? "null") as {
      version: number;
    };
    expect(stored.version).toBe(useAgentSessionStore.persist.getOptions().version);
  });
});

describe("drafts and queued first messages", () => {
  it("marks and graduates a draft", () => {
    store().markDraft("s1");
    expect(store().draftSessionIds.has("s1")).toBe(true);
    expect(store().freshDraftSessionIds.has("s1")).toBe(true);

    store().graduateDraft("s1");
    expect(store().draftSessionIds.has("s1")).toBe(false);
    expect(store().freshDraftSessionIds.has("s1")).toBe(false);
  });

  it("restores draft ownership without restoring the in-process freshness proof", async () => {
    localStorage.setItem(
      "lyra.agent-session",
      JSON.stringify({
        state: {
          openSessionIds: ["s1"],
          lastSessionId: "s1",
          draftSessionIds: ["s1"],
        },
        version: useAgentSessionStore.persist.getOptions().version,
      }),
    );

    await useAgentSessionStore.persist.rehydrate();

    expect(store().draftSessionIds).toEqual(new Set(["s1"]));
    expect(store().freshDraftSessionIds).toEqual(new Set());
  });

  it("graduating a session that isn't a draft changes nothing", () => {
    const before = store().draftSessionIds;
    store().graduateDraft("s1");
    expect(store().draftSessionIds).toBe(before);
  });

  it("takes a queued message exactly once", () => {
    const message = { input: agentTextInput("hello"), runOptions: {} };
    store().setPendingMessage("s1", message);

    expect(store().takePendingMessage("s1")).toEqual(message);
    expect(store().takePendingMessage("s1")).toBeUndefined();
  });

  it("prunes draft and pending refs when a session stops being open", () => {
    store().holdOpen("s1");
    store().markDraft("s1");
    store().setPendingMessage("s1", { input: agentTextInput("hi"), runOptions: {} });

    store().release("s1");

    expect(store().draftSessionIds.has("s1")).toBe(false);
    expect(store().freshDraftSessionIds.has("s1")).toBe(false);
    expect(store().takePendingMessage("s1")).toBeUndefined();
  });
});
