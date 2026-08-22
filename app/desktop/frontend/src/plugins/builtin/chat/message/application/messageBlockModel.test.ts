import { describe, expect, it } from "vitest";
import type {
  AgentRunView,
  ContentBlock,
  ToolCall,
} from "@/plugins/builtin/agent/public/viewState";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import {
  finalAnswerFollows,
  messageActionMaterialization,
  messageBlockRenderUnits,
  messageBlocksRenderInstant,
  narratedBlocks,
} from "./messageBlockModel";

const text = (text: string, status: "running" | "complete" = "complete"): ContentBlock => ({
  kind: "text",
  text,
  status,
});

const toolBlock = (toolCallId: string): ContentBlock => ({ kind: "tool", toolCallId });

const reasoning = (status: "running" | "complete" = "complete"): ContentBlock => ({
  kind: "reasoning",
  reasoningId: "r1",
  text: "why",
  status,
});

const tool = (
  id: string,
  name: string,
  safetyClass: ToolCall["safetyClass"] = "safe",
): ToolCall => ({
  id,
  runId: "run_1",
  name,
  fn: name,
  args: "",
  status: "ok",
  safetyClass,
});

describe("messageBlockRenderUnits", () => {
  it("coerces only non-tail running text blocks to complete", () => {
    const blocks = [text("first", "running"), text("last", "running")];

    expect(messageBlockRenderUnits(blocks, {})).toEqual([
      { kind: "block", block: text("first", "complete"), index: 0, superseded: true },
      { kind: "block", block: text("last", "running"), index: 1, superseded: false },
    ]);
  });

  it("keeps read-only tool grouping from the agent render planner", () => {
    const blocks = [toolBlock("a"), toolBlock("b"), text("done")];
    const tools = { a: tool("a", "read"), b: tool("b", "grep") };

    expect(messageBlockRenderUnits(blocks, tools)).toEqual([
      { kind: "toolGroup", tools: [tools.a, tools.b], superseded: true },
      { kind: "block", block: text("done"), index: 2, superseded: false },
    ]);
  });

  it("retains work-wave folding when the Runtime puts the final answer in the next row", () => {
    const blocks = [reasoning(), toolBlock("a"), toolBlock("b")];
    const tools = { a: tool("a", "read"), b: tool("b", "grep") };

    expect(messageBlockRenderUnits(blocks, tools, true)).toEqual([
      {
        kind: "wave",
        units: [
          { kind: "block", block: blocks[0], index: 0, superseded: true },
          { kind: "toolGroup", tools: [tools.a, tools.b], superseded: true },
        ],
      },
    ]);
  });

  // The rule the whole feature rests on: work is superseded by an answer that comes
  // AFTER it, which is why it is per unit rather than per message.
  it("marks work the answer already speaks for, and only that work", () => {
    const superseded = (blocks: ContentBlock[], tools = {}) =>
      messageBlockRenderUnits(blocks, tools).map((unit) =>
        unit.kind === "wave" ? "wave" : unit.superseded,
      );

    // Reasoning, then the answer: the reasoning is behind it.
    expect(superseded([reasoning(), text("answer")])).toEqual([true, false]);

    // A turn still working has nothing behind it yet.
    expect(superseded([reasoning("running")])).toEqual([false]);

    // Text still streaming counts as the answer having begun.
    expect(superseded([reasoning("running"), text("part", "running")])).toEqual([true, false]);

    // Interleaved: only the wave between the two answers folds.
    const tools = { a: tool("a", "shell", "exec") };
    expect(superseded([text("first"), toolBlock("a"), text("second")], tools)).toEqual([
      true,
      true,
      false,
    ]);

    // Trailing work after the last answer is the live wave — and the answer itself is
    // never superseded, because nothing that is text follows it.
    expect(superseded([text("first"), toolBlock("a")], tools)).toEqual([false, false]);
  });
});

describe("finalAnswerFollows", () => {
  const message = (phase: "commentary" | "finalAnswer", runId = "run_1") => ({
    id: `${phase}:${runId}`,
    role: "assistant" as const,
    runId,
    phase,
    blocks: [],
  });

  it("matches only adjacent work and final rows owned by the same Run", () => {
    expect(finalAnswerFollows(message("commentary"), message("finalAnswer"))).toBe(true);
    expect(finalAnswerFollows(message("commentary"), message("finalAnswer", "run_2"))).toBe(false);
    expect(finalAnswerFollows(message("finalAnswer"), message("finalAnswer"))).toBe(false);
    expect(finalAnswerFollows(message("commentary"), undefined)).toBe(false);
  });
});

describe("messageBlocksRenderInstant", () => {
  it("skips reveal animation only for user-authored messages", () => {
    expect(messageBlocksRenderInstant("user")).toBe(true);
    expect(messageBlocksRenderInstant("assistant")).toBe(false);
    expect(messageBlocksRenderInstant("system")).toBe(false);
  });
});

describe("messageActionMaterialization", () => {
  it("keeps a completed Item actionless while its exact Run still owns more output", () => {
    const streamingOwner = {
      ...row([text("temporarily complete")]),
      runOwner: { kind: "owned", runId: "run_1", status: "running" },
    } as TranscriptRow;

    expect(messageActionMaterialization(streamingOwner)).toBe("active");
  });

  it("does not treat an HITL pause or an unassigned assistant turn as terminal", () => {
    expect(
      messageActionMaterialization({
        ...row([text("waiting")]),
        runOwner: { kind: "owned", runId: "run_1", status: "waiting" },
      }),
    ).toBe("active");
    expect(
      messageActionMaterialization({
        ...row([text("recovering")]),
        runOwner: { kind: "unassigned" },
      }),
    ).toBe("active");
  });

  it("keeps a streaming tail and an HITL boundary actionless even without root attention", () => {
    expect(messageActionMaterialization(row([text("partial", "running")]))).toBe("active");
    expect(
      messageActionMaterialization(
        row([
          {
            kind: "question",
            status: "requires-action",
            questions: [],
          },
        ]),
      ),
    ).toBe("active");
  });

  it("keeps actions absent while a tool or delegated run still owns the turn", () => {
    const runningTool = tool("tool_1", "shell", "exec");
    runningTool.status = "running";
    expect(messageActionMaterialization(row([toolBlock("tool_1")], { tool_1: runningTool }))).toBe(
      "active",
    );

    expect(
      messageActionMaterialization({
        ...row([toolBlock("tool_1")]),
        facts: {
          toolCalls: {},
          delegatedRuns: {
            tool_1: [
              {
                run: agentRun("waiting"),
                messages: [],
              },
            ],
          },
        },
      }),
    ).toBe("active");
  });

  it("materializes actions for complete and recovery-incomplete output", () => {
    expect(messageActionMaterialization(row([text("done")]))).toBe("settled");
    expect(
      messageActionMaterialization(
        row([{ kind: "text", text: "preserved partial", status: "incomplete" }]),
      ),
    ).toBe("settled");
  });
});

// The plan was on screen twice: in the active surface above the composer, and
// again as the tool row that wrote it. A tool with a surface of its own has nothing
// left to say in the narrative — and it has to leave before the units are planned, or
// the counts and the grouping describe rows that are not there.
describe("narratedBlocks", () => {
  const standing = (name: string) => name === "set_plan";

  it("drops a tool whose surface already holds its outcome", () => {
    const blocks = [text("planning"), toolBlock("t_plan"), text("done")];
    const tools = { t_plan: tool("t_plan", "set_plan") };

    expect(narratedBlocks(blocks, tools, standing)).toEqual([blocks[0], blocks[2]]);
  });

  it("keeps every other tool, including the rest of the same family", () => {
    const blocks = [toolBlock("t_enter"), toolBlock("t_exit"), toolBlock("t_read")];
    const tools = {
      t_enter: tool("t_enter", "enter_plan_mode"),
      t_exit: tool("t_exit", "exit_plan_mode"),
      t_read: tool("t_read", "read"),
    };

    expect(narratedBlocks(blocks, tools, standing)).toEqual(blocks);
  });

  it("keeps a tool block whose call has not arrived yet", () => {
    const blocks = [toolBlock("t_unknown")];

    expect(narratedBlocks(blocks, {}, standing)).toEqual(blocks);
  });

  it("moves only an unanswered question to the composer request surface", () => {
    const pending: ContentBlock = {
      kind: "question",
      status: "requires-action",
      questions: [],
    };
    const answered: ContentBlock = { ...pending, status: "complete", answered: true };

    expect(narratedBlocks([pending], {}, standing)).toEqual([]);
    expect(narratedBlocks([answered], {}, standing)).toEqual([answered]);
  });

  it("closes the gap it leaves, so neighbours still group", () => {
    const blocks = [toolBlock("t_a"), toolBlock("t_plan"), toolBlock("t_b")];
    const tools = {
      t_a: tool("t_a", "read"),
      t_plan: tool("t_plan", "set_plan"),
      t_b: tool("t_b", "grep"),
    };

    const units = messageBlockRenderUnits(narratedBlocks(blocks, tools, standing), tools);

    expect(units).toEqual([
      { kind: "toolGroup", tools: [tools.t_a, tools.t_b], superseded: false },
    ]);
  });
});

function row(blocks: ContentBlock[], toolCalls: Record<string, ToolCall> = {}): TranscriptRow {
  return {
    message: {
      id: "message_1",
      role: "assistant" as const,
      runId: "run_1",
      blocks,
    },
    runOwner: { kind: "owned", runId: "run_1", status: "finished" },
    facts: { toolCalls, delegatedRuns: {} },
  };
}

function agentRun(status: AgentRunView["status"]): AgentRunView {
  return {
    id: "delegated_1",
    sessionId: "session_1",
    parentRunId: "run_1",
    rootRunId: "run_1",
    spawnedByItemId: "tool_1",
    status,
    activeSegmentId: status === "finished" ? null : "segment_1",
    outcome: status === "finished" ? { type: "completed" } : null,
    metrics: {
      steps: 0,
      activeDurationMillis: 0,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-08-17T00:00:00.000Z",
    finishedAt: status === "finished" ? "2026-08-17T00:00:01.000Z" : null,
  };
}
