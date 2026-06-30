import { describe, expect, it } from "vitest";
import type { OpenInterrupt } from "@/rpc";
import type { AgentViewState, ContentBlock, Message, RunError } from "./viewState";
import { INITIAL_VIEW_STATE } from "./viewState";
import {
  cancelRunningRun,
  dropMessage,
  relabelMessage,
  resolveInterrupt,
  setRunError,
} from "./viewMutations";

const time = "2026-06-03T00:00:00Z";

function view(partial: Partial<AgentViewState> = {}): AgentViewState {
  return {
    ...INITIAL_VIEW_STATE,
    run: { ...INITIAL_VIEW_STATE.run },
    messages: [],
    timeline: [],
    openInterrupts: [],
    ...partial,
  };
}

function message(id: string, blocks: ContentBlock[] = []): Message {
  return { id, role: "assistant", who: "Codex", time, blocks };
}

function approvalBlock(itemId: string): ContentBlock {
  return {
    kind: "approval",
    status: "requires-action",
    text: "Approve command?",
    command: "rm x",
    reason: "Needs confirmation",
    itemId,
    parentRunId: "run_1",
  };
}

function questionBlock(itemId: string): ContentBlock {
  return {
    kind: "question",
    status: "requires-action",
    itemId,
    parentRunId: "run_1",
    questions: [
      {
        id: "choice",
        question: "Which option?",
        header: "Choose",
        options: [{ label: "A", description: "Option A" }],
        multiSelect: false,
      },
    ],
  };
}

function openInterrupt(
  items: Array<{ itemId: string; type: "approval" | "question" }>,
): OpenInterrupt {
  return {
    parentRunId: "run_1",
    sessionId: "ses_1",
    createdAt: time,
    interrupts: items.map(({ itemId, type }) =>
      type === "approval"
        ? {
            type,
            itemId,
            payload: { tool: { name: "shell", arguments: { command: "rm x" } } },
          }
        : {
            type,
            itemId,
            payload: {
              question: {
                prompt: "Which option?",
                fields: [{ type: "text", name: "choice", label: "Which option?" }],
              },
            },
          },
    ),
  } as OpenInterrupt;
}

describe("view mutations - messages", () => {
  it("relabels an optimistic message without touching unrelated messages", () => {
    const original = view({
      messages: [message("local-1"), message("assistant-1")],
    });

    const next = relabelMessage(original, "local-1", "server-1");

    expect(next.messages.map((m) => m.id)).toEqual(["server-1", "assistant-1"]);
    expect(next.messages[1]).toBe(original.messages[1]);
  });

  it("does not relabel missing messages, existing target ids, or identical ids", () => {
    const original = view({
      messages: [message("local-1"), message("server-1")],
    });

    expect(relabelMessage(original, "missing", "server-2")).toBe(original);
    expect(relabelMessage(original, "local-1", "server-1")).toBe(original);
    expect(relabelMessage(original, "local-1", "local-1")).toBe(original);
  });

  it("drops a message by id and leaves unknown ids as no-ops", () => {
    const original = view({
      messages: [message("m1"), message("m2")],
    });

    const next = dropMessage(original, "m1");

    expect(next.messages.map((m) => m.id)).toEqual(["m2"]);
    expect(dropMessage(original, "missing")).toBe(original);
  });
});

describe("view mutations - run state", () => {
  it("sets and clears a run error only when the value changes", () => {
    const error: RunError = { message: "boom", code: "provider_error" };
    const original = view({ error });

    expect(setRunError(original, error)).toBe(original);
    expect(setRunError(original, null)).toMatchObject({ error: null });
  });

  it("cancels a running run and records a canceled run-end", () => {
    const original = view({
      run: { ...INITIAL_VIEW_STATE.run, running: true, runId: "run_1" },
    });

    const next = cancelRunningRun(original);

    expect(next.run.running).toBe(false);
    expect(next.timeline.at(-1)).toMatchObject({
      kind: "run-end",
      runId: "run_1",
      summary: "canceled",
    });
  });

  it("does not churn state when canceling an idle run", () => {
    const original = view();

    expect(cancelRunningRun(original)).toBe(original);
  });
});

describe("view mutations - interrupts", () => {
  it("settles an approval block, drops its interrupt, and stamps an approval result", () => {
    const original = view({
      messages: [message("assistant-1", [approvalBlock("tool_1")])],
      openInterrupts: [openInterrupt([{ itemId: "tool_1", type: "approval" }])],
    });

    const next = resolveInterrupt(original, "tool_1", { decision: "approved" });

    expect(next.messages[0]!.blocks[0]).toMatchObject({
      kind: "approval",
      status: "complete",
      decision: "approved",
    });
    expect(next.openInterrupts).toEqual([]);
    expect(next.timeline.at(-1)).toMatchObject({
      kind: "approval-result",
      refId: "tool_1",
      status: "approved",
    });
  });

  it("settles a question answer without stamping an approval result", () => {
    const answers = { choice: ["A"] };
    const original = view({
      messages: [message("assistant-1", [questionBlock("question_1")])],
      openInterrupts: [openInterrupt([{ itemId: "question_1", type: "question" }])],
    });

    const next = resolveInterrupt(original, "question_1", { answers });

    expect(next.messages[0]!.blocks[0]).toMatchObject({
      kind: "question",
      status: "complete",
      answered: true,
      answers,
    });
    expect(next.openInterrupts).toEqual([]);
    expect(next.timeline.some((entry) => entry.kind === "approval-result")).toBe(false);
  });

  it("removes only the resolved interrupt from a shared envelope", () => {
    const original = view({
      messages: [message("assistant-1", [approvalBlock("tool_1"), approvalBlock("tool_2")])],
      openInterrupts: [
        openInterrupt([
          { itemId: "tool_1", type: "approval" },
          { itemId: "tool_2", type: "approval" },
        ]),
      ],
    });

    const next = resolveInterrupt(original, "tool_1", { decision: "declined" });

    expect(next.openInterrupts).toHaveLength(1);
    expect(next.openInterrupts[0]!.interrupts.map((interrupt) => interrupt.itemId)).toEqual([
      "tool_2",
    ]);
    expect(next.messages[0]!.blocks[0]).toMatchObject({
      kind: "approval",
      status: "complete",
      decision: "declined",
    });
    expect(next.messages[0]!.blocks[1]).toMatchObject({
      kind: "approval",
      status: "requires-action",
    });
  });

  it("does not churn state or stamp results for an unknown item id", () => {
    const original = view({
      messages: [message("assistant-1", [approvalBlock("tool_1")])],
      openInterrupts: [openInterrupt([{ itemId: "tool_1", type: "approval" }])],
    });

    expect(resolveInterrupt(original, "missing", { decision: "approved" })).toBe(original);
  });
});
