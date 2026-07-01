import { describe, expect, it } from "vitest";
import type { ToolCall } from "@/protocol/run/viewState";
import {
  isReadOnlyTool,
  summarizeToolGroup,
  toolGroupNeedsAttention,
  toolIntent,
  toolMetaItems,
} from "./toolPresentation";

const tool = (overrides: Partial<ToolCall>): ToolCall => ({
  id: "tool-1",
  name: "shell",
  fn: "shell",
  args: "",
  status: "ok",
  ...overrides,
});

describe("toolPresentation", () => {
  it("projects tool args into a compact intent", () => {
    expect(toolIntent(tool({ fn: "read", args: JSON.stringify({ path: "src/App.tsx" }) }))).toEqual(
      {
        label: "Read",
        detail: "src/App.tsx",
      },
    );
  });

  it("ignores malformed args while keeping the tool label", () => {
    expect(toolIntent(tool({ fn: "my_tool", args: "{" }))).toEqual({ label: "my_tool" });
  });

  it("derives ordered meta badges", () => {
    expect(
      toolMetaItems(tool({ added: 3, removed: 2, hits: 7, exitCode: 1, status: "running" })),
    ).toEqual([
      { id: "added", label: "+3", tone: "success" },
      { id: "removed", label: "-2", tone: "negative" },
      { id: "hits", label: "7 matches", tone: "muted" },
      { id: "exit", label: "exit 1", tone: "negative" },
      { id: "live", label: "live", tone: "muted" },
    ]);
  });

  it("keeps read-only grouping conservative", () => {
    expect(isReadOnlyTool("read")).toBe(true);
    expect(isReadOnlyTool("lsp_diagnostics")).toBe(true);
    expect(isReadOnlyTool("edit")).toBe(false);
  });

  it("summarizes grouped tools by display bucket", () => {
    const tools = [
      tool({ id: "read", name: "read" }),
      tool({ id: "grep", name: "grep" }),
      tool({ id: "glob", name: "glob" }),
      tool({ id: "lsp", name: "lsp_diagnostics" }),
    ];
    expect(summarizeToolGroup(tools)).toBe("1 read · 2 search · 1 lookup");
  });

  it("marks groups needing attention only while running or failed", () => {
    expect(toolGroupNeedsAttention([tool({ status: "ok" })])).toBe(false);
    expect(toolGroupNeedsAttention([tool({ status: "running" })])).toBe(true);
    expect(toolGroupNeedsAttention([tool({ status: "err" })])).toBe(true);
  });
});
