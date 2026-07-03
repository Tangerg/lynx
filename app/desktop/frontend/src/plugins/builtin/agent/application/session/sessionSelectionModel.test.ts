import { describe, expect, it } from "vitest";
import {
  closeOpenSession,
  pruneSessionHandoffs,
  reconcileOpenSessions,
  selectOpenSession,
} from "./sessionSelectionModel";

describe("sessionSelectionModel", () => {
  it("selects a session and appends it only once", () => {
    const state = { activeSessionId: "s1", selectionEpoch: 2, openSessionIds: ["s1"] };

    expect(selectOpenSession(state, "s2")).toEqual({
      activeSessionId: "s2",
      selectionEpoch: 3,
      openSessionIds: ["s1", "s2"],
    });
    expect(selectOpenSession(state, "s1")).toEqual({
      activeSessionId: "s1",
      selectionEpoch: 3,
      openSessionIds: ["s1"],
    });
  });

  it("closes the active session by selecting its adjacent survivor", () => {
    expect(
      closeOpenSession({ activeSessionId: "s2", openSessionIds: ["s1", "s2", "s3"] }, "s2"),
    ).toEqual({
      activeSessionId: "s3",
      openSessionIds: ["s1", "s3"],
    });
    expect(
      closeOpenSession({ activeSessionId: "s3", openSessionIds: ["s1", "s2", "s3"] }, "s3"),
    ).toEqual({
      activeSessionId: "s2",
      openSessionIds: ["s1", "s2"],
    });
  });

  it("reconciles persisted open sessions against backend sessions and local drafts", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          draftSessionIds: new Set(["s3"]),
          openSessionIds: ["s1", "s2", "s3"],
        },
        ["s1"],
      ),
    ).toEqual({ activeSessionId: "s1", openSessionIds: ["s1", "s3"] });
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          draftSessionIds: new Set<string>(),
          openSessionIds: ["s1", "s2", "s3"],
        },
        ["s2", "s3"],
      ),
    ).toEqual({ activeSessionId: "s3", openSessionIds: ["s2", "s3"] });
  });

  it("returns null when persisted open sessions are already valid", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          draftSessionIds: new Set<string>(),
          openSessionIds: ["s1", "s2"],
        },
        ["s1", "s2"],
      ),
    ).toBeNull();
  });

  it("prunes draft and pending handoffs for closed sessions", () => {
    expect(
      pruneSessionHandoffs({
        draftSessionIds: new Set(["live", "closed"]),
        pendingMessages: { live: "keep", closed: "drop" },
        openSessionIds: ["live"],
      }),
    ).toEqual({
      draftSessionIds: new Set(["live"]),
      pendingMessages: { live: "keep" },
    });
  });
});
