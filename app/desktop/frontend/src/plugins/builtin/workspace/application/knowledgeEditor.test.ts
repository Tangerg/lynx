import { describe, expect, it } from "vitest";
import {
  commitKnowledgeSave,
  editKnowledge,
  isKnowledgeDirty,
  openedKnowledgeEditor,
  rebaseKnowledgeDraft,
  reconcileKnowledgeSnapshot,
  settleKnowledgeSave,
} from "./knowledgeEditor";

describe("knowledge editor", () => {
  it("uses the exact document as its editable baseline", () => {
    const state = openedKnowledgeEditor({
      content: "newer exact content",
      revision: "rev-1",
      updatedAt: "2026-08-12T00:00:00Z",
    });

    expect(state).toEqual({
      content: "newer exact content",
      draft: "newer exact content",
      revision: "rev-1",
      updatedAt: "2026-08-12T00:00:00Z",
    });
    expect(isKnowledgeDirty(state)).toBe(false);
  });

  it("does not erase an edit made while an older save is in flight", () => {
    const started = editKnowledge(
      openedKnowledgeEditor({ content: "old", revision: "rev-1" }),
      "first edit",
    );
    const editedAgain = editKnowledge(started, "second edit");
    const committed = commitKnowledgeSave(editedAgain, {
      content: "first edit",
      revision: "rev-2",
      updatedAt: "2026-08-12T00:01:00Z",
    });

    expect(committed).toEqual({
      content: "first edit",
      draft: "second edit",
      revision: "rev-2",
      updatedAt: "2026-08-12T00:01:00Z",
    });
    expect(isKnowledgeDirty(committed)).toBe(true);
  });

  it("adopts event-driven snapshots only while the editor is clean", () => {
    const clean = openedKnowledgeEditor({ content: "old", revision: "rev-1" });
    expect(reconcileKnowledgeSnapshot(clean, { content: "external", revision: "rev-2" })).toEqual({
      content: "external",
      draft: "external",
      revision: "rev-2",
    });

    const dirty = editKnowledge(clean, "local draft");
    expect(reconcileKnowledgeSnapshot(dirty, { content: "external", revision: "rev-2" })).toBe(
      dirty,
    );
  });

  it("rebases a conflicted draft onto the latest revision without erasing it", () => {
    const dirty = editKnowledge(
      openedKnowledgeEditor({ content: "old", revision: "rev-1" }),
      "local draft",
    );

    expect(
      rebaseKnowledgeDraft(dirty, {
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

  it("does not become clean on an obsolete save when a newer snapshot arrived in flight", () => {
    const saving = editKnowledge(
      openedKnowledgeEditor({ content: "old", revision: "rev-1" }),
      "saved edit",
    );

    expect(
      settleKnowledgeSave(
        saving,
        { content: "saved edit", revision: "rev-2" },
        { content: "remote after save", revision: "rev-3" },
      ),
    ).toEqual({
      content: "remote after save",
      draft: "remote after save",
      revision: "rev-3",
    });

    const editedAgain = editKnowledge(saving, "second local edit");
    expect(
      settleKnowledgeSave(
        editedAgain,
        { content: "saved edit", revision: "rev-2" },
        { content: "remote after save", revision: "rev-3" },
      ),
    ).toEqual({ content: "saved edit", draft: "second local edit", revision: "rev-2" });

    expect(
      settleKnowledgeSave(
        saving,
        { content: "saved edit", revision: "rev-2" },
        { content: "old", revision: "rev-1" },
      ),
    ).toEqual({ content: "saved edit", draft: "saved edit", revision: "rev-2" });
  });
});
