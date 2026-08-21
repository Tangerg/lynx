import { beforeAll, describe, expect, it } from "vitest";
import type { AgentSessionSnapshot } from "../ports/runtimeGateway";
import { projectAgentSessionSnapshot } from "./sessionSnapshot";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

const SESSION_ID = "ses_snapshot";
const ROOT_RUN_ID = "run_root";
const CHILD_RUN_ID = "run_child";
const RUNNING_CHILD_RUN_ID = "run_child_running";
const LOST_RUN_ID = "run_lost";
const usage = { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 };

beforeAll(async () => {
  const { default: foldPlugin } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPluginsForTest(foldPlugin);
});

describe("projectAgentSessionSnapshot", () => {
  it("rebuilds one complete source-owned view off-store", () => {
    const snapshot: AgentSessionSnapshot = {
      runs: [
        {
          id: LOST_RUN_ID,
          sessionId: SESSION_ID,
          status: "finished",
          parentRunId: null,
          rootRunId: LOST_RUN_ID,
          spawnedByItemId: null,
          activeSegmentId: null,
          createdAt: "2026-07-30T00:59:00.000Z",
          finishedAt: "2026-07-30T00:59:30.000Z",
          outcome: {
            type: "lost",
            error: {
              code: "run_lost",
              message: "runtime restarted before the run settled",
            },
          },
          metrics: { steps: 3, activeDurationMillis: 30, usage },
        },
        {
          id: ROOT_RUN_ID,
          sessionId: SESSION_ID,
          status: "running",
          parentRunId: null,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: null,
          activeSegmentId: "seg_root",
          outcome: null,
          finishedAt: null,
          createdAt: "2026-07-30T01:00:00.000Z",
          metrics: { steps: 2, activeDurationMillis: 20, usage },
        },
        {
          id: CHILD_RUN_ID,
          sessionId: SESSION_ID,
          status: "waiting",
          activeSegmentId: null,
          outcome: null,
          finishedAt: null,
          createdAt: "2026-07-30T01:00:01.000Z",
          metrics: { steps: 1, activeDurationMillis: 10, usage },
          parentRunId: ROOT_RUN_ID,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: "item_spawn",
        },
        {
          id: RUNNING_CHILD_RUN_ID,
          sessionId: SESSION_ID,
          status: "running",
          activeSegmentId: "seg_child_running",
          outcome: null,
          finishedAt: null,
          createdAt: "2026-07-30T01:00:01.500Z",
          metrics: { steps: 1, activeDurationMillis: 5, usage },
          parentRunId: ROOT_RUN_ID,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: "item_spawn_running",
        },
      ],
      items: [
        {
          type: "userMessage",
          id: "item_user",
          runId: ROOT_RUN_ID,
          status: "completed",
          createdAt: "2026-07-30T01:00:00.100Z",
          content: [{ type: "text", text: "delegate this" }],
        },
        {
          type: "agentMessage",
          id: "item_child_reply",
          runId: CHILD_RUN_ID,
          status: "completed",
          createdAt: "2026-07-30T01:00:01.100Z",
          content: [{ type: "text", text: "working" }],
        },
      ],
      pendingInterruptSets: [
        {
          rootRunId: ROOT_RUN_ID,
          sessionId: SESSION_ID,
          createdAt: "2026-07-30T01:00:02.000Z",
          interrupts: [
            {
              type: "approval",
              itemId: "item_approval",
              runId: CHILD_RUN_ID,
              payload: {
                tool: { name: "shell", arguments: { command: "npm test" } },
              },
            },
          ],
        },
      ],
      plan: {
        revision: 4,
        steps: [{ id: "step_1", text: "Verify", status: "active" }],
      },
    };

    const view = projectAgentSessionSnapshot(snapshot);

    expect(view.runsById[ROOT_RUN_ID]).toMatchObject({
      parentRunId: null,
      rootRunId: ROOT_RUN_ID,
      activeSegmentId: "seg_root",
    });
    expect(view.runsById[CHILD_RUN_ID]).toMatchObject({
      parentRunId: ROOT_RUN_ID,
      rootRunId: ROOT_RUN_ID,
      status: "waiting",
    });
    expect(view.runsById[RUNNING_CHILD_RUN_ID]).toMatchObject({
      parentRunId: ROOT_RUN_ID,
      rootRunId: ROOT_RUN_ID,
      status: "running",
      activeSegmentId: "seg_child_running",
    });
    expect(view.runsById[LOST_RUN_ID]).toMatchObject({
      status: "finished",
      outcome: {
        type: "lost",
        error: {
          code: "run_lost",
          message: "runtime restarted before the run settled",
        },
      },
    });
    expect(view.messages.map(({ id, runId }) => ({ id, runId }))).toEqual([
      { id: "item_user", runId: ROOT_RUN_ID },
      { id: "turn:item_child_reply", runId: CHILD_RUN_ID },
    ]);
    expect(
      view.messages.flatMap((message) => message.blocks).find((block) => block.kind === "approval"),
    ).toMatchObject({
      itemId: "item_approval",
      runId: ROOT_RUN_ID,
      status: "requires-action",
    });
    expect(view.pendingInterrupts).toEqual([
      {
        sessionId: SESSION_ID,
        runId: CHILD_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        interrupts: [{ itemId: "item_approval", kind: "approval" }],
      },
    ]);
    expect(view.plan).toMatchObject({ revision: 4 });
    expect(
      view.timeline.filter((entry) => entry.kind === "run-start").map((entry) => entry.runId),
    ).toEqual([LOST_RUN_ID, ROOT_RUN_ID, CHILD_RUN_ID, RUNNING_CHILD_RUN_ID]);
  });
});
