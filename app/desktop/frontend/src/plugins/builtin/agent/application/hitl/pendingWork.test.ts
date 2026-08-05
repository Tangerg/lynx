import { describe, expect, it } from "vitest";
import { pendingWorkItems, type PendingInterruptSetLike } from "./pendingWork";

function set(
  patch: Partial<PendingInterruptSetLike> & Pick<PendingInterruptSetLike, "interrupts">,
): PendingInterruptSetLike {
  return {
    sessionId: "ses_1",
    rootRunId: "run_1",
    createdAt: "2026-08-05T08:00:00.000Z",
    ...patch,
  };
}

describe("the queue of what is waiting on a person", () => {
  it("names the first ask and counts the rest, because resuming answers the set", () => {
    const items = pendingWorkItems([
      set({
        interrupts: [
          { type: "approval", payload: { tool: { name: "shell" } } },
          { type: "approval", payload: { tool: { name: "write" } } },
        ],
      }),
    ]);

    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ kind: "approval", subject: "shell", more: 1 });
  });

  it("reads a question's subject from its first field, not the question", () => {
    const items = pendingWorkItems([
      set({
        interrupts: [
          { type: "question", payload: { question: { fields: [{ prompt: "Which database?" }] } } },
        ],
      }),
    ]);

    expect(items[0]).toMatchObject({ kind: "question", subject: "Which database?", more: 0 });
  });

  // A toolResult interrupt is the runtime asking the runtime. Counting it would
  // put a row in a person's queue that no person can ever clear.
  it("drops a set that holds nothing a person can answer", () => {
    expect(pendingWorkItems([set({ interrupts: [{ type: "toolResult" }] })])).toEqual([]);
  });

  it("keeps the answerable ask when it shares a set with one that is not", () => {
    const items = pendingWorkItems([
      set({
        interrupts: [
          { type: "toolResult" },
          { type: "approval", payload: { tool: { name: "edit" } } },
        ],
      }),
    ]);

    expect(items[0]).toMatchObject({ subject: "edit", more: 0 });
  });

  it("keys a row by the set it resumes, so two sessions never collide", () => {
    const items = pendingWorkItems([
      set({ sessionId: "ses_a", interrupts: [{ type: "approval" }] }),
      set({ sessionId: "ses_b", interrupts: [{ type: "approval" }] }),
    ]);

    expect(new Set(items.map((item) => item.id)).size).toBe(2);
  });
});
