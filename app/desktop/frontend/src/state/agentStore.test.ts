// agentStore.resolveInterrupt — the optimistic HITL settle that runs the
// instant a continuation Run is sent (before its events stream back). Locks:
//   - the approval/question block flips out of requires-action by itemId
//   - the matching open interrupt is dropped
//   - an approval decision stamps an `approval-result` timeline entry (so the
//     run digest + Timeline view can pair it with its approval-request);
//     a question answer does NOT (questions have no timeline counterpart)

import { beforeEach, describe, expect, it } from "vitest";
import type { Item, RunOutcome, StreamEvent } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { useAgentStore } from "./agentStore";

const SID = "ses_1";

const item = (partial: Record<string, unknown>): Item =>
  ({ runId: "run_1", status: "running", createdAt: "2026-06-03T00:00:00Z", ...partial }) as Item;
const runStarted = (id: string, sessionId: string): StreamEvent =>
  ({ type: "run.started", run: { id, sessionId } }) as never;
const runFinished = (outcome: RunOutcome): StreamEvent => ({ type: "run.finished", outcome });
// Wrap a synthetic StreamEvent as a FoldEvent — no envelope runId, so the fold
// treats it as the root run (matching applyEvents' batch shape).
const fold = (event: StreamEvent) => ({ event });

// Drive the store to a state where `itemId` is an open interrupt of `kind`.
function seedInterrupt(kind: "approval" | "question", itemId: string): void {
  const store = useAgentStore.getState();
  store.resetSession(SID);
  store.applyEvents(
    SID,
    [
      runStarted("run_1", SID),
      kind === "approval"
        ? started(
            item({
              id: itemId,
              type: "toolCall",
              tool: { name: "bash", arguments: { command: "rm x" } },
            }),
          )
        : started(
            item({
              id: itemId,
              type: "question",
              question: {
                prompt: "Which?",
                fields: [{ type: "text", name: "f1", label: "Which?" }],
              },
            }),
          ),
      runFinished({
        type: "interrupt",
        interrupts: [
          kind === "approval"
            ? {
                itemId: itemId as never,
                type: "approval",
                payload: { tool: { name: "bash", arguments: { command: "rm x" } } },
              }
            : {
                itemId: itemId as never,
                type: "question",
                payload: {
                  question: {
                    prompt: "Which?",
                    fields: [{ type: "text", name: "f1", label: "Which?" }],
                  },
                },
              },
        ],
      }),
    ].map(fold),
  );
}
const started = (i: Item): StreamEvent => ({ type: "item.started", item: i });

const view = () => useAgentStore.getState().sessions[SID]!.view;

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/core-reducer");
  await loadPlugin(spec);
});

describe("agentStore.resolveInterrupt", () => {
  it("settles an approval, drops the open interrupt, and stamps approval-result", () => {
    seedInterrupt("approval", "tool_1");
    expect(view().openInterrupts).toHaveLength(1);

    useAgentStore.getState().resolveInterrupt(SID, "tool_1", { decision: "approved" });

    const block = view()
      .messages.flatMap((m) => m.blocks)
      .find((b) => b.kind === "approval");
    expect(block).toMatchObject({ status: "complete", decision: "approved" });
    expect(view().openInterrupts).toHaveLength(0);

    const tl = view().timeline.find((e) => e.kind === "approval-result");
    expect(tl).toMatchObject({ kind: "approval-result", refId: "tool_1", status: "approved" });
  });

  it("settles a question answer WITHOUT an approval-result entry", () => {
    seedInterrupt("question", "q_1");

    useAgentStore.getState().resolveInterrupt(SID, "q_1", { answered: true });

    const block = view()
      .messages.flatMap((m) => m.blocks)
      .find((b) => b.kind === "question");
    expect(block).toMatchObject({ status: "complete", answered: true });
    expect(view().openInterrupts).toHaveLength(0);
    expect(view().timeline.some((e) => e.kind === "approval-result")).toBe(false);
  });

  it("resolving one of several interrupts in an envelope keeps the siblings", () => {
    const store = useAgentStore.getState();
    store.resetSession(SID);
    store.applyEvents(
      SID,
      [
        runStarted("run_1", SID),
        started(
          item({
            id: "t1",
            type: "toolCall",
            tool: { name: "bash", arguments: { command: "rm a" } },
          }),
        ),
        started(
          item({
            id: "t2",
            type: "toolCall",
            tool: { name: "bash", arguments: { command: "rm b" } },
          }),
        ),
        runFinished({
          type: "interrupt",
          interrupts: [
            {
              itemId: "t1" as never,
              type: "approval",
              payload: { tool: { name: "bash", arguments: { command: "rm a" } } },
            },
            {
              itemId: "t2" as never,
              type: "approval",
              payload: { tool: { name: "bash", arguments: { command: "rm b" } } },
            },
          ],
        }),
      ].map(fold),
    );
    expect(view().openInterrupts[0]!.interrupts).toHaveLength(2);

    useAgentStore.getState().resolveInterrupt(SID, "t1", { decision: "approved" });

    // Envelope survives with only the unresolved sibling — not dropped whole.
    expect(view().openInterrupts).toHaveLength(1);
    expect(view().openInterrupts[0]!.interrupts.map((i) => i.itemId)).toEqual(["t2"]);
  });
});

describe("agentStore never resurrects a dropped session", () => {
  // Closing a tab mid-stream: the prune subscriber drops the slice
  // synchronously, but a late rAF flush / in-flight items.list / the unmount
  // cleanup nulling send-stop all run afterwards. None may re-seed a ghost
  // entry (prune won't fire again for an id no longer in tabIds → leak).
  it("applyEvents on an absent session is a no-op (no ghost entry)", () => {
    useAgentStore.getState().dropSession("ses_ghost");
    useAgentStore.getState().applyEvents("ses_ghost", [runStarted("run_x", "ses_ghost")].map(fold));
    expect(useAgentStore.getState().sessions["ses_ghost"]).toBeUndefined();
  });

  it("unmount-cleanup setters don't resurrect a dropped slice", () => {
    const store = useAgentStore.getState();
    store.resetSession(SID);
    expect(useAgentStore.getState().sessions[SID]).toBeDefined();
    store.dropSession(SID);
    // Order mirrors prod: prune drops the slice, THEN the effect cleanup runs.
    store.setSend(SID, null);
    store.setStop(SID, null);
    store.setResume(SID, null);
    expect(useAgentStore.getState().sessions[SID]).toBeUndefined();
  });
});

describe("agentStore.setError", () => {
  it("surfaces a channel-a failure on the banner; clearError dismisses it", () => {
    const store = useAgentStore.getState();
    store.resetSession(SID);
    store.setError(SID, { message: "session not found", code: "session_not_found" });
    expect(view().error).toMatchObject({ message: "session not found", code: "session_not_found" });
    useAgentStore.getState().clearError(SID);
    expect(view().error).toBeNull();
  });
});

describe("agentStore.relabelMessage", () => {
  const userMsg = (id: string): StreamEvent =>
    ({
      type: "item.completed",
      item: item({
        id,
        status: "completed",
        type: "userMessage",
        content: [{ type: "text", text: "hi" }],
      }),
    }) as never;

  it("renames an optimistic placeholder to the server id", () => {
    const store = useAgentStore.getState();
    store.resetSession(SID);
    store.applyEvents(SID, [userMsg("local-1")].map(fold));
    expect(view().messages.map((m) => m.id)).toEqual(["local-1"]);

    useAgentStore.getState().relabelMessage(SID, "local-1", "item_real");
    expect(view().messages.map((m) => m.id)).toEqual(["item_real"]);
  });

  it("is a no-op when the target id already exists (streamed item won the race)", () => {
    const store = useAgentStore.getState();
    store.resetSession(SID);
    store.applyEvents(SID, [userMsg("item_real"), userMsg("local-1")].map(fold));
    expect(view().messages).toHaveLength(2);

    useAgentStore.getState().relabelMessage(SID, "local-1", "item_real");
    // local-1 left as-is rather than collapsed into a duplicate-key clash.
    expect(view().messages.map((m) => m.id)).toEqual(["item_real", "local-1"]);
  });
});
