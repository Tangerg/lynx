// agentStore.resolveInterrupt — the optimistic HITL settle that runs the
// instant a continuation Run is sent (before its events stream back). Locks:
//   - the approval/question block flips out of requires-action by itemId
//   - the matching open interrupt is dropped
//   - an approval decision stamps an `approval-result` timeline entry (so the
//     run digest + Timeline view can pair it with its approval-request);
//     a question answer does NOT (questions have no timeline counterpart)

import { beforeEach, describe, expect, it } from "vitest";
import type { Item, RunEvent, RunRef, SegmentOutcome, StreamEvent } from "@/rpc";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { useAgentStore } from "./agentStore";
import { selectCurrentRootRun } from "../application/view/runTree";

const SID = "ses_1";

const item = (partial: Record<string, unknown>): Item =>
  ({ runId: "run_1", status: "running", createdAt: "2026-06-03T00:00:00Z", ...partial }) as Item;
const runStarted = (id: string, sessionId: string): StreamEvent => ({
  type: "segment.started",
  run: {
    id,
    sessionId,
    status: "running",
    activeSegmentId: `seg_${id}`,
    createdAt: "2026-06-03T00:00:00.000Z",
    metrics: { steps: 0, activeDurationMs: 0 },
    protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  },
});
const runFinished = (outcome: SegmentOutcome): StreamEvent => ({
  type: "segment.finished",
  outcome,
  metrics: { steps: 0, activeDurationMs: 0 },
});
let eventSequence = 0;
const fold = (event: StreamEvent): RunEvent => {
  const runId =
    event.type === "segment.started"
      ? event.run.id
      : event.type === "item.started" || event.type === "item.completed"
        ? event.item.runId
        : "run_1";
  return {
    event,
    eventId: `evt_store_${++eventSequence}`,
    runId,
    segmentId: `seg_${runId}`,
    timestamp: "2026-06-03T00:00:01.000Z",
  };
};

// Drive the store to a state where `itemId` is an open interrupt of `kind`.
function seedInterrupt(kind: "approval" | "question", itemId: string): void {
  const store = useAgentStore.getState();
  store.ensureSession(SID);
  store.applyRunEvents(
    SID,
    [
      runStarted("run_1", SID),
      kind === "approval"
        ? started(
            item({
              id: itemId,
              type: "toolCall",
              tool: { name: "shell", arguments: { command: "rm x" } },
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
                runId: "run_1" as never,
                type: "approval",
                payload: {
                  tool: { name: "shell", arguments: { command: "rm x" } },
                  rememberable: true,
                },
              }
            : {
                itemId: itemId as never,
                runId: "run_1" as never,
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
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const applyCompletedItems = (items: Item[]): void =>
  useAgentStore.getState().applyRunEvents(
    SID,
    items.map((value) => fold(completed(value))),
  );

const runRef = (partial: Partial<RunRef> = {}): RunRef => ({
  id: "run_1",
  sessionId: SID,
  status: "running",
  activeSegmentId: "seg_run_1",
  createdAt: "2026-06-03T00:00:00.000Z",
  metrics: { steps: 0, activeDurationMs: 0 },
  protocolProfile: { interruptTypes: [], requiredFeatures: [] },
  ...partial,
});

const view = () => useAgentStore.getState().sessions[SID]!.view;

beforeEach(async () => {
  useAgentStore.getState().dropSession(SID);
  const { default: spec } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(spec);
});

describe("agentStore.applyCancelResponse", () => {
  it("merges the exact root RunRef committed by the runtime", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.applyRunEvents(
      SID,
      [
        runStarted("run_1", SID),
        {
          type: "segment.progress",
          progress: { usage: { inputTokens: 1000, outputTokens: 200 } },
        } as StreamEvent,
      ].map(fold),
    );
    expect(selectCurrentRootRun(view())?.status).toBe("running");

    useAgentStore.getState().applyCancelResponse(SID, {
      type: "root",
      run: runRef({
        status: "finished",
        activeSegmentId: undefined,
        outcome: { type: "canceled" },
        finishedAt: "2026-06-03T00:00:02.000Z",
        metrics: {
          steps: 0,
          activeDurationMs: 1,
          usage: { inputTokens: 1000, outputTokens: 200 },
        },
      }),
    });

    expect(selectCurrentRootRun(view())?.status).toBe("finished");
    expect(view().timeline.at(-1)).toMatchObject({ kind: "run-end", summary: "canceled" });
  });

  it("merges a child and its post-cancel root without inventing sibling lifecycle", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.applyRunEvents(SID, [runStarted("run_1", SID), runStarted("run_sibling", SID)].map(fold));

    store.applyCancelResponse(SID, {
      type: "child",
      run: runRef({
        id: "run_child",
        parentRunId: "run_1",
        rootRunId: "run_1",
        spawnedByItemId: "item_spawn",
        status: "finished",
        activeSegmentId: undefined,
        outcome: { type: "canceled" },
        finishedAt: "2026-06-03T00:00:02.000Z",
      }),
      rootRun: runRef({
        activeSegmentId: "seg_resumed",
      }),
    });

    expect(view().runsById.run_child).toMatchObject({
      status: "finished",
      outcome: { type: "canceled" },
    });
    expect(view().runsById.run_1).toMatchObject({
      status: "running",
      activeSegmentId: "seg_resumed",
    });
    expect(view().runsById.run_sibling).toMatchObject({ status: "running" });
  });
});

describe("agentStore authoritative refresh", () => {
  it("keeps the current view visible until a complete snapshot commits", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.applyRunEvents(SID, [fold(runStarted("run_1", SID))]);
    const visible = view();

    const token = store.beginViewRefresh(SID, false)!;

    expect(view()).toBe(visible);
    expect(store.commitViewRefresh(SID, token, EMPTY_AGENT_SESSION_VIEW)).toBe(true);
    expect(view()).toBe(EMPTY_AGENT_SESSION_VIEW);
  });

  it("rejects a snapshot when a live event changed the projection after the read began", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    const token = store.beginViewRefresh(SID, false)!;
    store.applyRunEvents(SID, [fold(runStarted("run_live", SID))]);

    expect(store.commitViewRefresh(SID, token, EMPTY_AGENT_SESSION_VIEW)).toBe(false);
    expect(view().runsById.run_live).toMatchObject({ status: "running" });
  });

  it("lets the newest refresh supersede an older in-flight read", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    const older = store.beginViewRefresh(SID, false)!;
    const newer = store.beginViewRefresh(SID, false)!;

    expect(store.commitViewRefresh(SID, older, EMPTY_AGENT_SESSION_VIEW)).toBe(false);
    expect(
      store.commitViewRefresh(SID, newer, {
        ...EMPTY_AGENT_SESSION_VIEW,
        shared: { marker: "newest" },
      }),
    ).toBe(true);
    expect(view().shared).toEqual({ marker: "newest" });
  });

  it("invalidates queued stream events for a history rewrite without clearing the view", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.applyRunEvents(SID, [fold(runStarted("run_1", SID))]);
    const entry = useAgentStore.getState().sessions[SID]!;
    const visible = entry.view;

    store.beginViewRefresh(SID, true);

    const refreshed = useAgentStore.getState().sessions[SID]!;
    expect(refreshed.view).toBe(visible);
    expect(refreshed.viewEpoch).toBe(entry.viewEpoch + 1);
  });

  it("does not erase a materialized view when the session driver remounts", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.applyRunEvents(SID, [fold(runStarted("run_1", SID))]);
    const visible = view();

    store.ensureSession(SID);

    expect(view()).toBe(visible);
  });
});

describe("agentStore.resolveInterrupt", () => {
  it("settles an approval, drops the open interrupt, and stamps approval-result", () => {
    seedInterrupt("approval", "tool_1");
    expect(view().pendingInterrupts).toHaveLength(1);

    useAgentStore.getState().resolveInterrupt(SID, "tool_1", { decision: "approved" });

    const block = view()
      .messages.flatMap((m) => m.blocks)
      .find((b) => b.kind === "approval");
    expect(block).toMatchObject({ status: "complete", decision: "approved" });
    expect(view().pendingInterrupts).toHaveLength(0);

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
    expect(view().pendingInterrupts).toHaveLength(0);
    expect(view().timeline.some((e) => e.kind === "approval-result")).toBe(false);
  });

  it("resolving one of several interrupts in an envelope keeps the siblings", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.applyRunEvents(
      SID,
      [
        runStarted("run_1", SID),
        started(
          item({
            id: "t1",
            type: "toolCall",
            tool: { name: "shell", arguments: { command: "rm a" } },
          }),
        ),
        started(
          item({
            id: "t2",
            type: "toolCall",
            tool: { name: "shell", arguments: { command: "rm b" } },
          }),
        ),
        runFinished({
          type: "interrupt",
          interrupts: [
            {
              itemId: "t1" as never,
              runId: "run_1" as never,
              type: "approval",
              payload: {
                tool: { name: "shell", arguments: { command: "rm a" } },
                rememberable: true,
              },
            },
            {
              itemId: "t2" as never,
              runId: "run_1" as never,
              type: "approval",
              payload: {
                tool: { name: "shell", arguments: { command: "rm b" } },
                rememberable: true,
              },
            },
          ],
        }),
      ].map(fold),
    );
    expect(view().pendingInterrupts[0]!.interrupts).toHaveLength(2);

    useAgentStore.getState().resolveInterrupt(SID, "t1", { decision: "approved" });

    // Envelope survives with only the unresolved sibling — not dropped whole.
    expect(view().pendingInterrupts).toHaveLength(1);
    expect(view().pendingInterrupts[0]!.interrupts.map((i) => i.itemId)).toEqual(["t2"]);
  });
});

describe("agentStore never resurrects a dropped session", () => {
  // Closing a session mid-stream: the prune subscriber drops the slice
  // synchronously, but a late rAF flush / in-flight snapshot / the unmount
  // cleanup nulling send-stop all run afterwards. None may re-seed a ghost
  // entry (prune won't fire again for an id no longer in openSessionIds → leak).
  it("applyRunEvents on an absent session is a no-op (no ghost entry)", () => {
    useAgentStore.getState().dropSession("ses_ghost");
    useAgentStore
      .getState()
      .applyRunEvents("ses_ghost", [runStarted("run_x", "ses_ghost")].map(fold));
    expect(useAgentStore.getState().sessions["ses_ghost"]).toBeUndefined();
  });

  it("unmount-cleanup setters don't resurrect a dropped slice", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    expect(useAgentStore.getState().sessions[SID]).toBeDefined();
    store.dropSession(SID);
    // Order mirrors prod: prune drops the slice, THEN the effect cleanup runs.
    store.setSend(SID, null);
    store.setStop(SID, null);
    store.setResume(SID, null);
    expect(useAgentStore.getState().sessions[SID]).toBeUndefined();
  });
});

describe("agentStore.setCommandError", () => {
  it("surfaces a channel-a failure; clearProblem dismisses it", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    store.setCommandError(SID, {
      message: "session not found",
      code: "session_not_found",
    });
    expect(view().commandError).toMatchObject({
      message: "session not found",
      code: "session_not_found",
    });
    useAgentStore.getState().clearProblem(SID);
    expect(view().commandError).toBeNull();
  });
});

describe("agentStore.relabelMessage", () => {
  const userMsg = (id: string): Item =>
    item({
      id,
      status: "completed",
      type: "userMessage",
      content: [{ type: "text", text: "hi" }],
    });

  it("renames an optimistic placeholder to the server id", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    applyCompletedItems([userMsg("local-1")]);
    expect(view().messages.map((m) => m.id)).toEqual(["local-1"]);

    useAgentStore.getState().relabelMessage(SID, "local-1", "item_real");
    expect(view().messages.map((m) => m.id)).toEqual(["item_real"]);
  });

  it("is a no-op when the target id already exists (streamed item won the race)", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    applyCompletedItems([userMsg("item_real"), userMsg("local-1")]);
    expect(view().messages).toHaveLength(2);

    useAgentStore.getState().relabelMessage(SID, "local-1", "item_real");
    // local-1 left as-is rather than collapsed into a duplicate-key clash.
    expect(view().messages.map((m) => m.id)).toEqual(["item_real", "local-1"]);
  });
});

describe("agentStore.dropMessage", () => {
  const userMsg = (id: string): Item =>
    item({
      id,
      status: "completed",
      type: "userMessage",
      content: [{ type: "text", text: "hi" }],
    });

  it("removes a single message by id (optimistic steer rollback)", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    applyCompletedItems([userMsg("item_real"), userMsg("local-steer-1")]);
    expect(view().messages.map((m) => m.id)).toEqual(["item_real", "local-steer-1"]);

    useAgentStore.getState().dropMessage(SID, "local-steer-1");
    expect(view().messages.map((m) => m.id)).toEqual(["item_real"]);
  });

  it("is a no-op for an unknown id", () => {
    const store = useAgentStore.getState();
    store.ensureSession(SID);
    applyCompletedItems([userMsg("item_real")]);
    const before = view().messages;
    useAgentStore.getState().dropMessage(SID, "nope");
    expect(view().messages).toBe(before); // same reference — no churn
  });
});
