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

const tool = (id: string, name: string): ToolCall => ({
  id,
  name,
  fn: name,
  args: "",
  status: "ok",
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
      { kind: "block", block: text("first", "complete"), index: 0 },
      { kind: "block", block: text("last", "running"), index: 1 },
    ]);
  });

  it("keeps read-only tool grouping from the agent render planner", () => {
    const blocks = [toolBlock("a"), toolBlock("b"), text("done")];
    const tools = { a: tool("a", "read"), b: tool("b", "grep") };

    expect(messageBlockRenderUnits(blocks, tools)).toEqual([
      { kind: "toolGroup", tools: [tools.a, tools.b] },
      { kind: "block", block: text("done"), index: 2 },
    ]);
  });
});

describe("messageBlocksRenderInstant", () => {
  it("skips reveal animation only for user-authored messages", () => {
    expect(messageBlocksRenderInstant("user")).toBe(true);
    expect(messageBlocksRenderInstant("assistant")).toBe(false);
    expect(messageBlocksRenderInstant("system")).toBe(false);
  });
});
