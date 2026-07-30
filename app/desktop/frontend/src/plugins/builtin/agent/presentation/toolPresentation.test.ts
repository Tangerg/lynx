import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  isReadOnlyTool,
  summarizeToolGroup,
  toolGroupNeedsAttention,
  toolIntent,
  toolMetaItems,
} from "./toolPresentation";

const tool = ({ runId = "run_1", ...overrides }: Partial<ToolCall>): ToolCall => ({
  id: "tool-1",
  runId,
  name: "shell",
  fn: "shell",
  args: "",
  status: "ok",
  ...overrides,
});

describe("toolPresentation", () => {
  it("projects tool args into a compact intent", () => {
    expect(
      toolIntent(
        t,
        tool({ name: "read", fn: "read", args: JSON.stringify({ path: "src/App.tsx" }) }),
      ),
    ).toEqual({
      label: "Read",
      detail: "src/App.tsx",
    });
  });

  it("keeps a command verbatim even when it reads like a tool name", () => {
    // `fn` normally IS the command; only a projection that had nothing but the
    // tool's own name gets the table's word for it.
    expect(toolIntent(t, tool({ name: "shell", fn: "grep" })).label).toBe("grep");
    expect(toolIntent(t, tool({ name: "shell", fn: "shell" })).label).toBe("Shell");
  });

  it("ignores malformed args while keeping the tool label", () => {
    expect(toolIntent(t, tool({ fn: "my_tool", args: "{" }))).toEqual({ label: "my_tool" });
  });

  it("derives ordered meta badges", () => {
    expect(
      toolMetaItems(t, tool({ added: 3, removed: 2, hits: 7, exitCode: 1, status: "running" })),
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
    expect(summarizeToolGroup(t, tools)).toBe("1 read · 2 search · 1 lookup");
  });

  it("marks groups needing attention only while running or failed", () => {
    expect(toolGroupNeedsAttention([tool({ status: "ok" })])).toBe(false);
    expect(toolGroupNeedsAttention([tool({ status: "running" })])).toBe(true);
    expect(toolGroupNeedsAttention([tool({ status: "err" })])).toBe(true);
  });
});
