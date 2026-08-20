import { describe, expect, it } from "vitest";
import {
  closeOpenSession,
  openSession,
  pruneDraftSessions,
  reconcileOpenSessions,
} from "./sessionSelectionModel";

describe("sessionSelectionModel", () => {
  it("holds a session open, and only once", () => {
    expect(openSession(["s1"], "s2")).toEqual(["s1", "s2"]);
    // The same array back, so nothing downstream sees a change that isn't one.
    const open = ["s1"];
    expect(openSession(open, "s1")).toBe(open);
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

  it("reconciles persisted open sessions against backend sessions and fresh local creates", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          provisionalSessionIds: new Set(["s3"]),
          openSessionIds: ["s1", "s2", "s3"],
        },
        ["s1"],
      ),
    ).toEqual({ activeSessionId: "s1", openSessionIds: ["s1", "s3"] });
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "s1",
          provisionalSessionIds: new Set<string>(),
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
          provisionalSessionIds: new Set<string>(),
          openSessionIds: ["s1", "s2"],
        },
        ["s1", "s2"],
      ),
    ).toBeNull();
  });

  it("holds an authoritative deep-linked active session open during reconciliation", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "deep-link",
          provisionalSessionIds: new Set<string>(),
          openSessionIds: ["stale"],
        },
        ["deep-link"],
      ),
    ).toEqual({ activeSessionId: "deep-link", openSessionIds: ["deep-link"] });
  });

  it("does not treat persisted draft ownership as authoritative membership", () => {
    expect(
      reconcileOpenSessions(
        {
          activeSessionId: "draft-deleted-remotely",
          provisionalSessionIds: new Set<string>(),
          openSessionIds: ["draft-deleted-remotely"],
        },
        [],
      ),
    ).toEqual({ activeSessionId: "", openSessionIds: [] });
  });

  it("prunes draft ownership for closed sessions", () => {
    expect(
      pruneDraftSessions({
        draftSessionIds: new Set(["live", "closed"]),
        openSessionIds: ["live"],
      }),
    ).toEqual(new Set(["live"]));
  });
});
