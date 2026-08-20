import { describe, expect, it } from "vitest";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { planRenderUnits, waveStepCount, type MessageRenderUnit } from "./messageRenderUnits";

const text = (value: string, status: "running" | "complete" = "complete"): ContentBlock => ({
  kind: "text",
  text: value,
  status,
});

const reasoning = (status: "running" | "complete" = "complete"): ContentBlock => ({
  kind: "reasoning",
  reasoningId: "r1",
  text: "why",
  status,
});

const toolBlock = (toolCallId: string): ContentBlock => ({ kind: "tool", toolCallId });

const tool = (
  id: string,
  name: string,
  safetyClass: ToolCall["safetyClass"] = "safe",
  status: ToolCall["status"] = "ok",
): ToolCall => ({
  id,
  runId: "run_1",
  name,
  fn: name,
  args: "",
  status,
  safetyClass,
});

const TOOLS: Record<string, ToolCall> = {
  read: tool("read", "read"),
  grep: tool("grep", "grep"),
  shell: tool("shell", "shell", "exec"),
  edit: tool("edit", "edit", "write"),
};

/** Shape only — what nests inside what, which is the whole of this planner's job. */
const shape = (units: MessageRenderUnit[]): unknown =>
  units.map((unit) => {
    if (unit.kind === "wave") return { wave: shape(unit.units) };
    if (unit.kind === "toolGroup") return `group(${unit.tools.length})`;
    return unit.block.kind;
  });

describe("planRenderUnits", () => {
  // The rule the transcript's readability rests on: work · answer · work · answer,
  // with each run of work behind an answer folded to one row.
  it("folds each run of work that already has an answer after it", () => {
    const units = planRenderUnits(
      [
        reasoning(),
        toolBlock("shell"),
        text("first answer"),
        reasoning(),
        toolBlock("edit"),
        text("second answer"),
      ],
      TOOLS,
    );

    expect(shape(units)).toEqual([
      { wave: ["reasoning", "tool"] },
      "text",
      { wave: ["reasoning", "tool"] },
      "text",
    ]);
  });

  // The run in flight is the one the reader is watching, and it is the only one that
  // must not be folded away underneath them.
  it("leaves the run still in flight unfolded", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("shell"), text("answer"), reasoning("running"), toolBlock("edit")],
      TOOLS,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "tool"] }, "text", "reasoning", "tool"]);
  });

  it("counts a streaming answer as an answer", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("shell"), text("part", "running")],
      TOOLS,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "tool"] }, "text"]);
  });

  // Wrapping something that already folds itself only adds a level to open through.
  it("does not wrap a run that plans to a single row", () => {
    expect(shape(planRenderUnits([reasoning(), text("answer")], TOOLS))).toEqual([
      "reasoning",
      "text",
    ]);
    expect(
      shape(planRenderUnits([toolBlock("read"), toolBlock("grep"), text("answer")], TOOLS)),
    ).toEqual(["group(2)", "text"]);
  });

  it("keeps read-only grouping inside a folded run", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("read"), toolBlock("grep"), text("answer")],
      TOOLS,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "group(2)"] }, "text"]);
  });

  it("folds a work-only message when its final answer follows in the next row", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("read"), toolBlock("grep"), toolBlock("edit")],
      TOOLS,
      true,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "group(2)", "tool"] }]);
  });

  // A request for a decision may never be folded: a turn whose approval is hidden
  // behind a summary row waits forever on a click nobody knows to make.
  it("never folds a block that is asking the reader for something", () => {
    const approval: ContentBlock = {
      kind: "approval",
      status: "complete",
      itemId: "item_1",
      toolName: "shell",
      command: "rm -rf build",
      reason: "destructive",
    };
    const units = planRenderUnits([reasoning(), approval, toolBlock("shell"), text("done")], TOOLS);

    expect(shape(units)).toEqual(["reasoning", "approval", "tool", "text"]);
  });

  it("renders one request surface when an approval owns the same pending tool call", () => {
    const approval: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: "shell",
      runId: "run_1",
      toolName: "shell",
      command: "pwd && ls",
      reason: "Runs commands in the workspace",
    };
    const tools = {
      shell: tool("shell", "shell", "exec", "requires-action"),
    };

    expect(shape(planRenderUnits([toolBlock("shell"), approval], tools))).toEqual(["approval"]);
  });
});

// What "the answer has begun" means, which decides whether the thinking and the tool
// work fold away. It is not "a text block exists": `item.started` creates the answer's
// block empty, so that reading folded a running reasoning block the instant the model
// opened its reply — leaving a one-line row with no preview in it and nothing yet on
// screen that could have replaced it.
describe("planRenderUnits · what counts as the answer", () => {
  const superseded = (blocks: ContentBlock[]) =>
    planRenderUnits(blocks, TOOLS).map((unit) => (unit.kind === "wave" ? "wave" : unit.superseded));

  it("keeps thinking open while the answer's block is still empty", () => {
    expect(superseded([reasoning("running"), text("", "running")])).toEqual([false, false]);
    expect(superseded([reasoning("running"), text("\n  ", "running")])).toEqual([false, false]);
  });

  it("folds thinking as soon as the answer has words", () => {
    expect(superseded([reasoning("running"), text("A", "running")])).toEqual([true, false]);
  });

  it("keeps tool work open on an empty answer block too", () => {
    expect(superseded([toolBlock("shell"), text("", "running")])).toEqual([false, false]);
    expect(superseded([toolBlock("shell"), text("done")])).toEqual([true, false]);
  });
});

// Cases moved here from `chat/message/ui/renderUnits.test.ts`, which asserted this
// ring's planner from another one — two homes for one function's contract.
describe("planRenderUnits · read-only grouping", () => {
  const tb = toolBlock;

  it("folds 2+ adjacent read-only tools into one group", () => {
    expect(planRenderUnits([tb("read"), tb("grep")], TOOLS)).toEqual([
      { kind: "toolGroup", tools: [TOOLS.read, TOOLS.grep], superseded: false },
    ]);
  });

  it("keeps a lone read-only tool as its own block", () => {
    const blocks = [tb("read")];
    expect(planRenderUnits(blocks, TOOLS)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
    ]);
  });

  it("never groups side-effecting tools", () => {
    const blocks = [tb("edit"), tb("shell")];
    expect(planRenderUnits(blocks, TOOLS)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
      { kind: "block", block: blocks[1], index: 1, superseded: false },
    ]);
  });

  it("breaks a run on a side-effecting tool and preserves original indices", () => {
    const blocks = [tb("shell"), tb("read"), tb("grep"), tb("edit")];
    expect(planRenderUnits(blocks, TOOLS)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
      { kind: "toolGroup", tools: [TOOLS.read, TOOLS.grep], superseded: false },
      { kind: "block", block: blocks[3], index: 3, superseded: false },
    ]);
  });

  it("groups lsp lookups", () => {
    const tools = { one: tool("one", "lsp"), two: tool("two", "lsp") };
    expect(planRenderUnits([tb("one"), tb("two")], tools)).toEqual([
      { kind: "toolGroup", tools: [tools.one, tools.two], superseded: false },
    ]);
  });

  it("drops a HITL-question tool's shadow row when its question block is present", () => {
    const question: ContentBlock = { kind: "question", status: "complete", questions: [] };
    const tools = { ask: tool("ask", "ask_user") };
    expect(planRenderUnits([tb("ask"), question], tools)).toEqual([
      { kind: "block", block: question, index: 1, superseded: false },
    ]);
  });

  it("keeps a HITL-question tool row when no question block accompanies it", () => {
    const blocks = [tb("ask")];
    const tools = { ask: tool("ask", "ask_user") };
    expect(planRenderUnits(blocks, tools)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
    ]);
  });

  it("treats an unresolved tool block as a plain block", () => {
    const blocks = [tb("read"), tb("missing")];
    expect(planRenderUnits(blocks, TOOLS)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
      { kind: "block", block: blocks[1], index: 1, superseded: false },
    ]);
  });
});

describe("waveStepCount", () => {
  // The first version of the row counted tool calls only, so a round of two commands
  // and two conclusions said it held two things.
  it("counts every step inside, thinking included and groups unpacked", () => {
    const wave = (blocks: ContentBlock[]) => {
      const first = planRenderUnits(blocks, TOOLS)[0];
      if (first?.kind !== "wave") throw new Error("expected a folded wave");
      return waveStepCount(first.units);
    };

    // One thought + a group of two reads.
    expect(wave([reasoning(), toolBlock("read"), toolBlock("grep"), text("answer")])).toBe(3);
    // Two thoughts around two side-effecting calls, none of them grouped.
    expect(
      wave([reasoning(), toolBlock("shell"), reasoning(), toolBlock("edit"), text("answer")]),
    ).toBe(4);
  });
});
