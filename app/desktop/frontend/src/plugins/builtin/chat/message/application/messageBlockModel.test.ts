import { describe, expect, it } from "vitest";
import type { ContentBlock, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { CitationSource } from "@/plugins/sdk";
import {
  messageBlockRenderUnits,
  messageBlocksRenderInstant,
  messageCitations,
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

describe("messageCitations", () => {
  it("flattens citation sources and owns continuous indices", () => {
    const blocks = [text("See [1] and [2].")];
    const sources: CitationSource[] = [
      () => [{ index: 99, domain: "a.test", title: "A", snippet: "first" }],
      () => [
        { index: 42, domain: "b.test", title: "B", snippet: "second" },
        { index: 43, domain: "c.test", title: "C", snippet: "third" },
      ],
    ];

    expect(messageCitations(blocks, sources)).toEqual([
      { index: 1, domain: "a.test", title: "A", snippet: "first" },
      { index: 2, domain: "b.test", title: "B", snippet: "second" },
      { index: 3, domain: "c.test", title: "C", snippet: "third" },
    ]);
  });
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

describe("messageBlocksRenderInstant", () => {
  it("skips reveal animation only for user-authored messages", () => {
    expect(messageBlocksRenderInstant("user")).toBe(true);
    expect(messageBlocksRenderInstant("assistant")).toBe(false);
    expect(messageBlocksRenderInstant("system")).toBe(false);
  });
});
