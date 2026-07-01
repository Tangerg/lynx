import { describe, expect, it } from "vitest";
import {
  emptyComposerDraft,
  mirrorComposerDraft,
  nextComposerHistory,
  previousComposerHistory,
  pruneComposerArchives,
  pushComposerHistory,
} from "./draftArchive";

describe("composer draft archive", () => {
  it("mirrors the active draft into the archive", () => {
    const state = {
      activeSid: "s1",
      drafts: {},
      value: "",
      images: [],
      pastes: [],
    };

    expect(mirrorComposerDraft(state, { value: "hello" })).toEqual({
      value: "hello",
      images: [],
      pastes: [],
      drafts: { s1: { value: "hello", images: [], pastes: [] } },
    });
  });

  it("keeps scratch, active, and open session archives", () => {
    const draft = emptyComposerDraft();
    const pruned = pruneComposerArchives(
      {
        activeSid: "s2",
        drafts: { "": draft, s1: draft, s2: draft, stale: draft },
        history: { s1: ["one"], s2: ["two"], stale: ["gone"] },
      },
      new Set(["s1"]),
    );

    expect(Object.keys(pruned.drafts)).toEqual(["", "s1", "s2"]);
    expect(Object.keys(pruned.history)).toEqual(["s1", "s2"]);
  });

  it("recalls history without duplicating consecutive entries", () => {
    const initial = {
      activeSid: "s1",
      history: {},
      histIndex: -1,
      histDraft: "",
      value: "draft",
    };
    const first = pushComposerHistory(initial, "hello", 50)!;
    const second = pushComposerHistory({ ...initial, history: first.history }, "hello", 50)!;
    const state = { ...initial, history: second.history };

    expect(state.history.s1).toEqual(["hello"]);
    expect(previousComposerHistory(state)).toEqual({
      value: "hello",
      histIndex: 0,
      histDraft: "draft",
    });
    expect(nextComposerHistory({ ...state, histIndex: 0, histDraft: "draft" })).toEqual({
      value: "draft",
      histIndex: -1,
    });
  });
});
