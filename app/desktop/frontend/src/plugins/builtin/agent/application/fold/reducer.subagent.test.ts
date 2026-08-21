import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentEventEnvelope as RunEvent,
  AgentItem as Item,
  AgentRunFact as RunRef,
  AgentStreamEvent as StreamEvent,
} from "@/plugins/sdk";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import { reduceAgentEvent } from "./reducer";
import { foldRunSnapshot } from "./runSnapshot";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const METRICS = {
  steps: 0,
  activeDurationMillis: 0,
  usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
};

function runningRun(
  id: string,
  segmentId: string,
  lineage?: {
    parentRunId: string;
    rootRunId: string;
    spawnedByItemId: string;
  },
): RunRef {
  return {
    id,
    sessionId: "ses_1",
    status: "running",
    activeSegmentId: segmentId,
    parentRunId: lineage?.parentRunId ?? null,
    rootRunId: lineage?.rootRunId ?? id,
    spawnedByItemId: lineage?.spawnedByItemId ?? null,
    outcome: null,
    finishedAt: null,
    createdAt: `2026-06-03T00:00:0${id.length}.000Z`,
    metrics: METRICS,
  };
}

function envelope(eventId: string, runId: string, segmentId: string, event: StreamEvent): RunEvent {
  return {
    event,
    eventId,
    runId,
    segmentId,
    timestamp: "2026-06-03T00:01:00.000Z",
  };
}

function started(eventId: string, run: RunRef): RunEvent {
  return envelope(eventId, run.id, run.activeSegmentId!, {
    type: "segment.started",
    run,
  });
}

function progress(eventId: string, runId: string, segmentId: string, step: number): RunEvent {
  return envelope(eventId, runId, segmentId, {
    type: "segment.progress",
    progress: { step, activity: `${runId} work` },
  });
}

function finished(eventId: string, runId: string, segmentId: string): RunEvent {
  return envelope(eventId, runId, segmentId, {
    type: "segment.finished",
    outcome: { type: "completed" },
    metrics: { ...METRICS, steps: 7, activeDurationMillis: 50 },
  });
}

function itemStarted(eventId: string, item: Item, segmentId: string): RunEvent {
  return envelope(eventId, item.runId, segmentId, {
    type: "item.started",
    item,
  });
}

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(spec);
});

describe("reducer — source-owned Run tree", () => {
  it("keeps child interrupt provenance while binding the complete set to its root", () => {
    const root = runningRun("root", "seg_root");
    const childA = runningRun("child_a", "seg_a", {
      parentRunId: root.id,
      rootRunId: root.id,
      spawnedByItemId: "spawn_a",
    });
    const childB = runningRun("child_b", "seg_b", {
      parentRunId: root.id,
      rootRunId: root.id,
      spawnedByItemId: "spawn_b",
    });
    let view = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, started("evt_root_start", root));
    view = reduceAgentEvent(view, started("evt_a_start", childA));
    view = reduceAgentEvent(view, started("evt_b_start", childB));

    view = reduceAgentEvent(
      view,
      envelope("evt_root_wait", root.id, "seg_root", {
        type: "segment.finished",
        outcome: {
          type: "interrupt",
          interrupts: [
            {
              type: "approval",
              itemId: "approval_a",
              runId: childA.id,
              payload: { tool: { name: "shell", arguments: { command: "make test" } } },
            },
            {
              type: "question",
              itemId: "question_b",
              runId: childB.id,
              payload: {
                question: {
                  fields: [{ type: "text", prompt: "Release name?", header: "Name" }],
                },
              },
            },
          ],
        },
        metrics: METRICS,
      }),
    );

    expect(view.pendingInterrupts).toEqual([
      {
        sessionId: "ses_1",
        runId: childA.id,
        rootRunId: root.id,
        interrupts: [{ itemId: "approval_a", kind: "approval" }],
      },
      {
        sessionId: "ses_1",
        runId: childB.id,
        rootRunId: root.id,
        interrupts: [{ itemId: "question_b", kind: "question" }],
      },
    ]);
    const blocks = view.messages.flatMap((message) => message.blocks);
    expect(blocks.find((block) => block.kind === "approval")).toMatchObject({
      itemId: "approval_a",
      runId: root.id,
    });
    expect(blocks.find((block) => block.kind === "question")).toMatchObject({
      itemId: "question_b",
      runId: root.id,
    });
    expect(view.messages.map((message) => message.runId)).toEqual([childA.id, childB.id]);
  });

  it("projects root, siblings, and nested descendants without cross-run overwrite", () => {
    const root = runningRun("root", "seg_root");
    const childA = runningRun("child_a", "seg_a", {
      parentRunId: root.id,
      rootRunId: root.id,
      spawnedByItemId: "spawn_a",
    });
    const childB = runningRun("child_b", "seg_b", {
      parentRunId: root.id,
      rootRunId: root.id,
      spawnedByItemId: "spawn_b",
    });
    const nested = runningRun("nested", "seg_nested", {
      parentRunId: childA.id,
      rootRunId: root.id,
      spawnedByItemId: "spawn_nested",
    });

    let view = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, started("evt_root_start", root));
    view = reduceAgentEvent(view, started("evt_a_start", childA));
    view = reduceAgentEvent(view, started("evt_b_start", childB));
    view = reduceAgentEvent(view, started("evt_nested_start", nested));
    view = reduceAgentEvent(view, progress("evt_root_progress", root.id, "seg_root", 1));
    view = reduceAgentEvent(view, progress("evt_a_progress", childA.id, "seg_a", 2));
    view = reduceAgentEvent(view, progress("evt_b_progress", childB.id, "seg_b", 3));
    view = reduceAgentEvent(view, progress("evt_nested_progress", nested.id, "seg_nested", 4));

    expect(view.runsById).toMatchObject({
      root: {
        parentRunId: null,
        rootRunId: "root",
        progress: { step: 1, activity: "root work" },
      },
      child_a: {
        parentRunId: "root",
        rootRunId: "root",
        progress: { step: 2, activity: "child_a work" },
      },
      child_b: {
        parentRunId: "root",
        rootRunId: "root",
        progress: { step: 3, activity: "child_b work" },
      },
      nested: {
        parentRunId: "child_a",
        rootRunId: "root",
        progress: { step: 4, activity: "nested work" },
      },
    });

    view = reduceAgentEvent(view, finished("evt_nested_finish", nested.id, "seg_nested"));
    expect(view.runsById.nested).toMatchObject({
      status: "finished",
      outcome: { type: "completed" },
    });
    expect(view.runsById.root?.status).toBe("running");
    expect(view.runsById.child_a?.status).toBe("running");
    expect(view.runsById.child_b?.status).toBe("running");
  });

  it("keeps interleaved messages, plans, tools, and assistant turns owned by source Run", () => {
    const root = runningRun("root", "seg_root");
    const child = runningRun("child", "seg_child", {
      parentRunId: root.id,
      rootRunId: root.id,
      spawnedByItemId: "spawn_child",
    });
    let view = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, started("evt_root_start", root));
    view = reduceAgentEvent(view, started("evt_child_start", child));

    const rootMessage = {
      id: "root_message",
      runId: root.id,
      type: "agentMessage",
      status: "running",
      createdAt: "2026-06-03T00:02:00.000Z",
      content: [],
    } as Item;
    const childMessage = {
      id: "child_message",
      runId: child.id,
      type: "agentMessage",
      status: "running",
      createdAt: "2026-06-03T00:02:01.000Z",
      content: [],
    } as Item;
    const rootTool = {
      id: "root_tool",
      runId: root.id,
      type: "toolCall",
      status: "running",
      startedAt: "2026-06-03T00:02:03.000Z",
      tool: { name: "shell", arguments: { command: "pwd", description: "Print the cwd" } },
    } as Item;

    view = reduceAgentEvent(view, itemStarted("evt_child_message", childMessage, "seg_child"));
    view = reduceAgentEvent(view, itemStarted("evt_root_message", rootMessage, "seg_root"));
    view = reduceAgentEvent(view, itemStarted("evt_root_tool", rootTool, "seg_root"));

    expect(view.messages.map(({ runId }) => runId)).toEqual(["child", "root"]);
    expect(view.assistantTurnByRunId).toEqual({
      child: "turn:child_message",
      root: "turn:root_message",
    });
    expect(view.toolCalls.root_tool).toMatchObject({
      runId: "root",
      fn: "Print the cwd",
      command: "pwd",
    });
    expect(
      view.timeline.find((entry) => entry.id === "timeline:evt_root_tool:tool-start"),
    ).toMatchObject({ runId: "root", refId: "root_tool" });
  });

  it("converges live terminal folding with the durable RunRef snapshot", () => {
    const root = runningRun("root", "seg_root");
    const terminalEvent = finished("evt_root_finish", root.id, "seg_root");
    const startedView = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, started("evt_root_start", root));
    const live = reduceAgentEvent(startedView, terminalEvent);
    const duplicate = reduceAgentEvent(live, terminalEvent);
    expect(duplicate).toBe(live);

    const cold = foldRunSnapshot(EMPTY_AGENT_SESSION_VIEW, {
      ...root,
      status: "finished",
      activeSegmentId: null,
      outcome: { type: "completed" },
      metrics: { ...METRICS, steps: 7, activeDurationMillis: 50 },
      finishedAt: terminalEvent.timestamp,
    });
    expect(cold.runsById.root).toEqual(live.runsById.root);
  });

  it("hydrates the last authoritative prompt footprint from a durable Run snapshot", () => {
    const cold = foldRunSnapshot(EMPTY_AGENT_SESSION_VIEW, {
      ...runningRun("root", "seg_root"),
      status: "finished",
      activeSegmentId: null,
      outcome: { type: "completed" },
      contextTokens: 87_900,
      finishedAt: "2026-06-03T00:01:00.000Z",
    });

    expect(cold.runsById.root?.progress).toEqual({ contextTokens: 87_900 });
  });

  it("does not let duplicate or late segment.started regress a newer Run state", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const root = runningRun("root", "seg_root");
    const startEvent = started("evt_root_start", root);
    const startedView = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, startEvent);
    const progressed = reduceAgentEvent(
      startedView,
      progress("evt_root_progress", root.id, "seg_root", 3),
    );

    expect(reduceAgentEvent(progressed, startEvent)).toBe(progressed);
    const terminal = reduceAgentEvent(progressed, finished("evt_root_finish", root.id, "seg_root"));
    expect(reduceAgentEvent(terminal, startEvent)).toBe(terminal);

    const late = reduceAgentEvent(terminal, started("evt_late_start", root));
    expect(late).toBe(terminal);
    expect(error).toHaveBeenCalledWith(
      expect.stringContaining('stream handler "segment.started"'),
      expect.objectContaining({
        message: expect.stringContaining("agent.fold.runStatusMismatch"),
      }),
    );
    error.mockRestore();
  });

  it("does not let a second segment start while another segment is running", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const root = runningRun("root", "seg_root");
    const startedView = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, started("evt_root_start", root));
    const conflicting = reduceAgentEvent(
      startedView,
      started("evt_conflicting_start", runningRun("root", "seg_other")),
    );

    expect(conflicting).toBe(startedView);
    expect(error).toHaveBeenCalledWith(
      expect.stringContaining('stream handler "segment.started"'),
      expect.objectContaining({
        message: expect.stringContaining("agent.fold.segmentMismatch"),
      }),
    );
    error.mockRestore();
  });

  it("fails closed when an Item owner disagrees with the event envelope", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const root = runningRun("root", "seg_root");
    const startedView = reduceAgentEvent(EMPTY_AGENT_SESSION_VIEW, started("evt_root_start", root));
    const foreignItem = {
      id: "foreign_message",
      runId: "child",
      type: "agentMessage",
      status: "running",
      createdAt: "2026-06-03T00:03:00.000Z",
      content: [],
    } as Item;

    const next = reduceAgentEvent(
      startedView,
      envelope("evt_bad_owner", root.id, "seg_root", { type: "item.started", item: foreignItem }),
    );

    expect(next).toBe(startedView);
    expect(error).toHaveBeenCalledWith(
      expect.stringContaining('stream handler "item.started"'),
      expect.objectContaining({
        message: expect.stringContaining("agent.fold.itemSourceMismatch"),
      }),
    );
    error.mockRestore();
  });
});
