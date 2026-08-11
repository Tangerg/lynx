import { describe, expect, it } from "vitest";
import {
  commitKnowledgeSave,
  editKnowledge,
  isKnowledgeDirty,
  openedKnowledgeEditor,
} from "./knowledgeEditor";

describe("knowledge editor", () => {
  it("uses the exact document as its editable baseline", () => {
    const state = openedKnowledgeEditor({
      content: "newer exact content",
      updatedAt: "2026-08-12T00:00:00Z",
    });

    expect(state).toEqual({
      content: "newer exact content",
      draft: "newer exact content",
      updatedAt: "2026-08-12T00:00:00Z",
    });
    expect(isKnowledgeDirty(state)).toBe(false);
  });

  it("does not erase an edit made while an older save is in flight", () => {
    const started = editKnowledge(openedKnowledgeEditor({ content: "old" }), "first edit");
    const editedAgain = editKnowledge(started, "second edit");
    const committed = commitKnowledgeSave(editedAgain, "first edit");

    expect(committed).toEqual({ content: "first edit", draft: "second edit" });
    expect(isKnowledgeDirty(committed)).toBe(true);
  });
});
