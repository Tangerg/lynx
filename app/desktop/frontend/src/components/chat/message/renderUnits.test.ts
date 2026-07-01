// planRenderUnits — folds adjacent read-only tool calls into groups while
// leaving every other block (and side-effecting tools) untouched, preserving
// original block indices for the caller's streaming-text coercion.

import { describe, expect, it } from "vitest";
import type { ContentBlock, ToolCall } from "@/protocol/run/viewState";
import { planRenderUnits } from "@/plugins/builtin/agent/presentation/messageRenderUnits";

const tb = (toolCallId: string): ContentBlock => ({ kind: "tool", toolCallId });
const tool = (id: string, name: string, status: ToolCall["status"] = "ok"): ToolCall => ({
  id,
  name,
  fn: name,
  args: "",
  status,
});

describe("planRenderUnits", () => {
  it("folds 2+ adjacent read-only tools into one group", () => {
    const blocks = [tb("a"), tb("b")];
    const tools = { a: tool("a", "read"), b: tool("b", "grep") };
    expect(planRenderUnits(blocks, tools)).toEqual([
      { kind: "toolGroup", tools: [tools.a, tools.b] },
    ]);
  });

  it("keeps a lone read-only tool as its own block", () => {
    const blocks = [tb("a")];
    const tools = { a: tool("a", "read") };
    expect(planRenderUnits(blocks, tools)).toEqual([{ kind: "block", block: blocks[0], index: 0 }]);
  });

  it("never groups side-effecting tools", () => {
    const blocks = [tb("a"), tb("b")];
    const tools = { a: tool("a", "edit"), b: tool("b", "write") };
    expect(planRenderUnits(blocks, tools)).toEqual([
      { kind: "block", block: blocks[0], index: 0 },
      { kind: "block", block: blocks[1], index: 1 },
    ]);
  });

  it("breaks a run on a side-effecting tool and preserves original indices", () => {
    const blocks = [tb("x"), tb("a"), tb("b"), tb("y")];
    const tools = {
      x: tool("x", "shell"),
      a: tool("a", "read"),
      b: tool("b", "read"),
      y: tool("y", "shell"),
    };
    expect(planRenderUnits(blocks, tools)).toEqual([
      { kind: "block", block: blocks[0], index: 0 },
      { kind: "toolGroup", tools: [tools.a, tools.b] },
      { kind: "block", block: blocks[3], index: 3 },
    ]);
  });

  it("groups lsp lookups", () => {
    const blocks = [tb("a"), tb("b")];
    const tools = { a: tool("a", "lsp"), b: tool("b", "lsp_diagnostics") };
    expect(planRenderUnits(blocks, tools)).toEqual([
      { kind: "toolGroup", tools: [tools.a, tools.b] },
    ]);
  });

  it("drops a HITL-question tool's shadow row when its question block is present", () => {
    const q: ContentBlock = { kind: "question", status: "complete", questions: [] };
    const blocks = [tb("ask"), q];
    const tools = { ask: tool("ask", "ask_user", "err") }; // drained → incomplete → err
    expect(planRenderUnits(blocks, tools)).toEqual([{ kind: "block", block: q, index: 1 }]);
  });

  it("keeps a HITL-question tool row when no question block accompanies it", () => {
    const blocks = [tb("ask")];
    const tools = { ask: tool("ask", "ask_user", "ok") }; // non-parking runtime
    expect(planRenderUnits(blocks, tools)).toEqual([{ kind: "block", block: blocks[0], index: 0 }]);
  });

  it("treats an unresolved tool block as a plain block", () => {
    const blocks = [tb("a"), tb("missing")];
    const tools = { a: tool("a", "read") };
    expect(planRenderUnits(blocks, tools)).toEqual([
      { kind: "block", block: blocks[0], index: 0 },
      { kind: "block", block: blocks[1], index: 1 },
    ]);
  });
});
