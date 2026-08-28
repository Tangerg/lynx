import { describe, expect, it } from "vitest";
import type { RunEvent, RunProtocolProfile, RunRef } from "@/rpc";
import {
  runtimeAgentEvent,
  runtimeCancelResult,
  runtimeItem,
  runtimePendingInterruptSet,
  runtimeRunFact,
} from "./runtimeAgentFacts";

const PROFILE: RunProtocolProfile = { interruptTypes: [], requiredFeatures: [] };
const METRICS = { steps: 2, activeDurationMillis: 25 };

function runningRoot(patch: Partial<RunRef> = {}): RunRef {
  return {
    id: "run_root",
    sessionId: "ses_1",
    status: "running",
    activeSegmentId: "seg_1",
    createdAt: "2026-08-12T08:00:00.000Z",
    metrics: METRICS,
    protocolProfile: PROFILE,
    ...patch,
  };
}

function event(value: RunEvent["event"]): RunEvent {
  return {
    event: value,
    eventId: "evt_1",
    runId: "run_root",
    segmentId: "seg_1",
    timestamp: "2026-08-12T08:00:01.000Z",
  };
}

describe("Runtime → Agent fact adapter", () => {
  it("normalizes a live root Run into a complete product fact", () => {
    expect(
      runtimeRunFact(
        runningRoot({
          provider: "openai",
          model: "gpt-5.6-sol",
          reasoningEffort: "high",
          contextTokens: 198_000,
        }),
      ),
    ).toEqual({
      id: "run_root",
      sessionId: "ses_1",
      parentRunId: null,
      rootRunId: "run_root",
      spawnedByItemId: null,
      status: "running",
      activeSegmentId: "seg_1",
      outcome: null,
      modelSelection: {
        provider: "openai",
        model: "gpt-5.6-sol",
        reasoningEffort: "high",
      },
      metrics: {
        steps: 2,
        activeDurationMillis: 25,
        usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
      },
      contextTokens: 198_000,
      createdAt: "2026-08-12T08:00:00.000Z",
      finishedAt: null,
    });
  });

  it("translates durable child lineage, terminal errors, usage, and cancel results", () => {
    const child: RunRef = {
      id: "run_child",
      sessionId: "ses_1",
      status: "finished",
      createdAt: "2026-08-12T08:00:00.000Z",
      finishedAt: "2026-08-12T08:00:02.000Z",
      parentRunId: "run_root",
      rootRunId: "run_root",
      spawnedByItemId: "item_spawn",
      outcome: {
        type: "lost",
        error: { type: "run_lost", detail: "runtime restarted" },
      },
      metrics: {
        steps: 4,
        activeDurationMillis: 50,
        usage: { inputTokens: 11, outputTokens: 7, cacheReadTokens: 3, costUsd: 0.01 },
      },
      protocolProfile: PROFILE,
    };

    const mapped = runtimeRunFact(child);
    expect(mapped).toMatchObject({
      parentRunId: "run_root",
      rootRunId: "run_root",
      spawnedByItemId: "item_spawn",
      activeSegmentId: null,
      outcome: {
        type: "lost",
        error: { code: "run_lost", message: "runtime restarted" },
      },
      metrics: { usage: { cacheReadTokens: 3, costUsd: 0.01 } },
    });
    expect(runtimeCancelResult({ type: "child", rootRun: runningRoot(), run: child })).toEqual({
      type: "child",
      rootRun: runtimeRunFact(runningRoot()),
      run: mapped,
    });
  });

  it("translates every payload-bearing event before SDK dispatch", () => {
    expect(
      runtimeItem({
        type: "agentMessage",
        id: "item_answer",
        runId: "run_root",
        status: "completed",
        createdAt: "2026-08-12T08:00:01.000Z",
        phase: "finalAnswer",
        content: [{ type: "text", text: "Done." }],
      }),
    ).toEqual({
      type: "agentMessage",
      id: "item_answer",
      runId: "run_root",
      status: "completed",
      createdAt: "2026-08-12T08:00:01.000Z",
      phase: "finalAnswer",
      content: [{ type: "text", text: "Done." }],
    });

    const tool = runtimeItem({
      type: "toolCall",
      id: "item_tool",
      runId: "run_root",
      status: "incomplete",
      startedAt: "2026-08-12T08:00:00.000Z",
      finishedAt: "2026-08-12T08:00:01.000Z",
      safetyClass: "exec",
      approvalDecision: "deny",
      tool: { name: "shell", arguments: { command: "false" } },
      error: { type: "tool_failed", detail: "exit 1" },
    });
    expect(tool).toMatchObject({
      type: "toolCall",
      safetyClass: "exec",
      approvalDecision: "declined",
      error: { code: "tool_failed", message: "exit 1" },
    });

    expect(
      runtimeAgentEvent(
        event({
          type: "segment.progress",
          progress: { step: 3, usage: { inputTokens: 5, outputTokens: 2 } },
        }),
      ).event,
    ).toEqual({
      type: "segment.progress",
      progress: {
        step: 3,
        usage: { inputTokens: 5, outputTokens: 2, cacheReadTokens: 0 },
      },
    });

    expect(
      runtimeAgentEvent(
        event({
          type: "plan.updated",
          plan: {
            sessionId: "ses_1",
            revision: 6,
            steps: [{ id: "step_1", description: "Verify", status: "in_progress" }],
            updatedAt: "2026-08-17T00:00:00Z",
          },
        }),
      ).event,
    ).toEqual({
      type: "plan.updated",
      plan: {
        revision: 6,
        steps: [{ id: "step_1", text: "Verify", status: "active" }],
      },
    });
  });

  it("translates the install-wide HITL snapshot without publishing wire payloads", () => {
    expect(
      runtimePendingInterruptSet({
        rootRunId: "run_root",
        sessionId: "ses_1",
        createdAt: "2026-08-12T08:00:01.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_tool",
            runId: "run_child",
            payload: {
              reason: "writes a file",
              rememberable: true,
              risk: "high",
              tool: { name: "apply_patch", arguments: { patch: "*** Begin Patch\n…" } },
            },
          },
        ],
      }),
    ).toEqual({
      rootRunId: "run_root",
      sessionId: "ses_1",
      createdAt: "2026-08-12T08:00:01.000Z",
      interrupts: [
        {
          type: "approval",
          itemId: "item_tool",
          runId: "run_child",
          payload: {
            reason: "writes a file",
            rememberable: true,
            tool: { name: "apply_patch", arguments: { patch: "*** Begin Patch\n…" } },
          },
        },
      ],
    });
  });

  it.each([
    ["status", runningRoot({ status: undefined }), "statusMissing"],
    ["creation time", runningRoot({ createdAt: undefined }), "createdAtMissing"],
    [
      "complete child lineage",
      runningRoot({ spawnedByItemId: "item_spawn", parentRunId: "run_root" }),
      "childLineageMissing",
    ],
    ["root lineage absence", runningRoot({ rootRunId: "run_root" }), "rootLineagePresent"],
    ["running segment", runningRoot({ activeSegmentId: undefined }), "activeSegmentMissing"],
    ["complete model identity", runningRoot({ provider: "openai" }), "modelSelectionIncomplete"],
    [
      "reasoning model ownership",
      runningRoot({ reasoningEffort: "high" }),
      "reasoningWithoutModel",
    ],
    [
      "waiting segment absence",
      runningRoot({ status: "waiting", activeSegmentId: "seg_1" }),
      "unexpectedActiveSegment",
    ],
    [
      "terminal facts",
      runningRoot({ status: "finished", activeSegmentId: undefined }),
      "terminalFactsMissing",
    ],
    [
      "non-terminal facts absence",
      runningRoot({
        status: "waiting",
        activeSegmentId: undefined,
        finishedAt: "2026-08-12T08:00:02.000Z",
        outcome: { type: "completed" },
      }),
      "unexpectedTerminalFacts",
    ],
  ])("rejects malformed Runtime %s at the Adapter boundary", (_case, run, code) => {
    expect(() => runtimeRunFact(run)).toThrow(`agent.adapter.run.${code}`);
  });
});
