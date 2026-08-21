import { describe, expect, it } from "vitest";
import type { Interrupt, PendingInterruptSet } from "@/rpc";
import { pendingWorkItems } from "./pendingWork";

// The wire's own shapes, so a fixture cannot describe an interrupt the runtime
// could never send — which is what a hand-written mirror of the payload allowed.
function approval(tool: string): Interrupt {
  return {
    type: "approval",
    itemId: `item_${tool}`,
    runId: "run_1",
    payload: { tool: { name: tool, arguments: {} } },
  };
}

function question(...prompts: string[]): Interrupt {
  return {
    type: "question",
    itemId: "item_question",
    runId: "run_1",
    payload: { question: { fields: prompts.map((prompt) => ({ type: "text", prompt })) } },
  };
}

function set(
  patch: Partial<PendingInterruptSet> & Pick<PendingInterruptSet, "interrupts">,
): PendingInterruptSet {
  return {
    sessionId: "ses_1",
    rootRunId: "run_1",
    createdAt: "2026-08-05T08:00:00.000Z",
    ...patch,
  };
}

describe("the queue of what is waiting on a person", () => {
  it("names the first ask and counts the rest, because resuming answers the set", () => {
    const items = pendingWorkItems([set({ interrupts: [approval("shell"), approval("write")] })]);

    expect(items).toHaveLength(1);
    expect(items[0]).toMatchObject({ kind: "approval", subject: "shell", more: 1 });
  });

  it("reads a question's subject from its first field, not the question", () => {
    const items = pendingWorkItems([set({ interrupts: [question("Which database?")] })]);

    expect(items[0]).toMatchObject({ kind: "question", subject: "Which database?", more: 0 });
  });

  it("keys a row by the set it resumes, so two sessions never collide", () => {
    const items = pendingWorkItems([
      set({ sessionId: "ses_a", interrupts: [approval("shell")] }),
      set({ sessionId: "ses_b", interrupts: [approval("shell")] }),
    ]);

    expect(new Set(items.map((item) => item.id)).size).toBe(2);
  });
});
