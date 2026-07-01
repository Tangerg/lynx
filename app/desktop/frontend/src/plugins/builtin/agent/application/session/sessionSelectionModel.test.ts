import { describe, expect, it } from "vitest";
import {
  closeSessionTab,
  pruneSessionHandoffs,
  reconcileSessionTabs,
  selectSessionTab,
} from "./sessionSelectionModel";

describe("sessionSelectionModel", () => {
  it("selects a session and appends it only once", () => {
    const state = { activeSessionId: "s1", selectionEpoch: 2, tabIds: ["s1"] };

    expect(selectSessionTab(state, "s2")).toEqual({
      activeSessionId: "s2",
      selectionEpoch: 3,
      tabIds: ["s1", "s2"],
    });
    expect(selectSessionTab(state, "s1")).toEqual({
      activeSessionId: "s1",
      selectionEpoch: 3,
      tabIds: ["s1"],
    });
  });

  it("closes the active tab by selecting its adjacent survivor", () => {
    expect(closeSessionTab({ activeSessionId: "s2", tabIds: ["s1", "s2", "s3"] }, "s2")).toEqual({
      activeSessionId: "s3",
      tabIds: ["s1", "s3"],
    });
    expect(closeSessionTab({ activeSessionId: "s3", tabIds: ["s1", "s2", "s3"] }, "s3")).toEqual({
      activeSessionId: "s2",
      tabIds: ["s1", "s2"],
    });
  });

  it("reconciles persisted tabs against backend sessions and local drafts", () => {
    expect(
      reconcileSessionTabs(
        {
          activeSessionId: "s1",
          draftSessionIds: new Set(["s3"]),
          tabIds: ["s1", "s2", "s3"],
        },
        ["s1"],
      ),
    ).toEqual({ activeSessionId: "s1", tabIds: ["s1", "s3"] });
    expect(
      reconcileSessionTabs(
        {
          activeSessionId: "s1",
          draftSessionIds: new Set<string>(),
          tabIds: ["s1", "s2", "s3"],
        },
        ["s2", "s3"],
      ),
    ).toEqual({ activeSessionId: "s3", tabIds: ["s2", "s3"] });
  });

  it("returns null when persisted session tabs are already valid", () => {
    expect(
      reconcileSessionTabs(
        {
          activeSessionId: "s1",
          draftSessionIds: new Set<string>(),
          tabIds: ["s1", "s2"],
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
        tabIds: ["live"],
      }),
    ).toEqual({
      draftSessionIds: new Set(["live"]),
      pendingMessages: { live: "keep" },
    });
  });
});
