import { beforeAll, describe, expect, it } from "vitest";
import type { AgentSessionSnapshot } from "../ports/runtimeGateway";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { projectAgentSessionSnapshot } from "./sessionSnapshot";

const SESSION_ID = "ses_snapshot";
const ROOT_RUN_ID = "run_root";
const CHILD_RUN_ID = "run_child";
const RUNNING_CHILD_RUN_ID = "run_child_running";
const LOST_RUN_ID = "run_lost";

beforeAll(async () => {
  const { default: foldPlugin } = await import("@/plugins/builtin/agent/public/foldPlugin");
  await loadPlugin(foldPlugin);
});

describe("projectAgentSessionSnapshot", () => {
  it("rebuilds one complete source-owned view off-store", () => {
    const snapshot: AgentSessionSnapshot = {
      runs: [
        {
          id: LOST_RUN_ID,
          sessionId: SESSION_ID,
          status: "finished",
          createdAt: "2026-07-30T00:59:00.000Z",
          finishedAt: "2026-07-30T00:59:30.000Z",
          outcome: {
            type: "error",
            error: {
              type: "run_lost",
              detail: "runtime restarted before the run settled",
            },
          },
          metrics: { steps: 3, activeDurationMs: 30 },
          protocolProfile: { interruptTypes: [], requiredFeatures: [] },
        },
        {
          id: ROOT_RUN_ID,
          sessionId: SESSION_ID,
          status: "running",
          activeSegmentId: "seg_root",
          createdAt: "2026-07-30T01:00:00.000Z",
          metrics: { steps: 2, activeDurationMs: 20 },
          protocolProfile: { interruptTypes: ["approval"], requiredFeatures: ["subagents"] },
        },
        {
          id: CHILD_RUN_ID,
          sessionId: SESSION_ID,
          status: "waiting",
          createdAt: "2026-07-30T01:00:01.000Z",
          metrics: { steps: 1, activeDurationMs: 10 },
          protocolProfile: { interruptTypes: ["approval"], requiredFeatures: [] },
          parentRunId: ROOT_RUN_ID,
          rootRunId: ROOT_RUN_ID,
          spawnedByItemId: "item_spawn",
        },
        {
          id: RUNNING_CHILD_RUN_ID,
          sessionId: SESSION_ID,
          status: "running",
          activeSegmentId: "seg_child_running",
          createdAt: "2026-07-30T01:00:01.500Z",
          metrics: { steps: 1, activeDurationMs: 5 },
          protocolProfile: { interruptTypes: [], requiredFeatures: [] },
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
      state: {
        type: "todos",
        sessionId: SESSION_ID,
        revision: 4,
        updatedAt: "2026-07-30T01:00:03.000Z",
        todos: [{ id: "todo_1", text: "Verify", status: "in_progress" }],
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
        type: "error",
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
      runId: CHILD_RUN_ID,
      status: "requires-action",
    });
    expect(view.pendingInterrupts).toEqual([
      {
        sessionId: SESSION_ID,
        runId: CHILD_RUN_ID,
        interrupts: [{ itemId: "item_approval", kind: "approval" }],
      },
    ]);
    expect(view.shared.todos).toMatchObject({ revision: 4 });
    expect(
      view.timeline.filter((entry) => entry.kind === "run-start").map((entry) => entry.runId),
    ).toEqual([LOST_RUN_ID, ROOT_RUN_ID, CHILD_RUN_ID, RUNNING_CHILD_RUN_ID]);
  });
});
