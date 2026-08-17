import { describe, expect, it } from "vitest";
import { KnowledgeDraft } from "./knowledge";

describe("KnowledgeDraft", () => {
  it("uses the exact document as its editable baseline", () => {
    const draft = KnowledgeDraft.open({
      content: "newer exact content",
      revision: "rev-1",
      updatedAt: "2026-08-12T00:00:00Z",
    });

    expect(draft).toEqual({
      content: "newer exact content",
      draft: "newer exact content",
      revision: "rev-1",
      updatedAt: "2026-08-12T00:00:00Z",
    });
    expect(draft.dirty).toBe(false);
  });

  it("does not erase an edit made while an older save is in flight", () => {
    const saving = KnowledgeDraft.open({ content: "old", revision: "rev-1" }).edit("first edit");
    const editedAgain = saving.edit("second edit");
    const committed = editedAgain.settleSave(
      {
        content: "first edit",
        revision: "rev-2",
        updatedAt: "2026-08-12T00:01:00Z",
      },
      { content: "old", revision: "rev-1" },
    );

    expect(committed).toEqual({
      content: "first edit",
      draft: "second edit",
      revision: "rev-2",
      updatedAt: "2026-08-12T00:01:00Z",
    });
    expect(committed.dirty).toBe(true);
  });

  it("adopts event-driven snapshots only while the draft is clean", () => {
    const clean = KnowledgeDraft.open({ content: "old", revision: "rev-1" });
    expect(clean.reconcile({ content: "external", revision: "rev-2" })).toEqual({
      content: "external",
      draft: "external",
      revision: "rev-2",
      updatedAt: undefined,
    });

    const dirty = clean.edit("local draft");
    expect(dirty.reconcile({ content: "external", revision: "rev-2" })).toBe(dirty);
  });

  it("rebases a conflicted draft without erasing user intent", () => {
    const dirty = KnowledgeDraft.open({ content: "old", revision: "rev-1" }).edit("local draft");

    expect(
      dirty.rebase({
        content: "external",
        revision: "rev-2",
        updatedAt: "2026-08-12T00:02:00Z",
      }),
    ).toEqual({
      content: "external",
      draft: "local draft",
      revision: "rev-2",
      updatedAt: "2026-08-12T00:02:00Z",
    });
  });

  it("does not become clean on an obsolete save when a newer snapshot arrived", () => {
    const saving = KnowledgeDraft.open({ content: "old", revision: "rev-1" }).edit("saved edit");

    expect(
      saving.settleSave(
        { content: "saved edit", revision: "rev-2" },
        { content: "remote after save", revision: "rev-3" },
      ),
    ).toEqual({
      content: "remote after save",
      draft: "remote after save",
      revision: "rev-3",
      updatedAt: undefined,
    });

    const editedAgain = saving.edit("second local edit");
    expect(
      editedAgain.settleSave(
        { content: "saved edit", revision: "rev-2" },
        { content: "remote after save", revision: "rev-3" },
      ),
    ).toEqual({
      content: "saved edit",
      draft: "second local edit",
      revision: "rev-2",
      updatedAt: undefined,
    });
  });
});
