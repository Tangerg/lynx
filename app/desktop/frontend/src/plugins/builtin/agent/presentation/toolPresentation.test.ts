import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  toolDiffStat,
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
      label: { kind: "text", value: "Read" },
      // A path says so, because only the projection that chose it knows which
      // case this is — the row truncates a path from the other end.
      detail: { kind: "path", value: "src/App.tsx" },
    });
  });

  it("says so when the LABEL is the path, which is where the file tools put it", () => {
    // `fn` carries the path for read/edit/write (the fold bakes the key argument
    // into the label), so the kind has to travel with it or the row clips the
    // filename off the only part a reader was looking for.
    expect(
      toolIntent(t, tool({ name: "edit", fn: "app/runtime/store.go", fnKind: "path" })).label,
    ).toEqual({ kind: "path", value: "app/runtime/store.go" });
  });

  it("marks a pattern as text, not as a path", () => {
    expect(
      toolIntent(t, tool({ name: "grep", fn: "grep", args: JSON.stringify({ pattern: "a/b" }) }))
        .detail,
    ).toEqual({ kind: "text", value: "a/b" });
  });

  it("prefers the step a plan is on over anything in its arguments", () => {
    expect(
      toolIntent(t, tool({ name: "set_plan", fn: "set_plan", step: "Write the fix" })),
    ).toEqual({
      label: { kind: "text", value: "Update the plan" },
      detail: { kind: "text", value: "Write the fix" },
    });
  });

  it("keeps a command verbatim even when it reads like a tool name", () => {
    // `fn` normally IS the command; only a projection that had nothing but the
    // tool's own name gets the table's word for it.
    expect(toolIntent(t, tool({ name: "shell", fn: "grep" })).label.value).toBe("grep");
    expect(toolIntent(t, tool({ name: "shell", fn: "shell" })).label.value).toBe("Shell");
  });

  it("ignores malformed args while keeping the tool label", () => {
    expect(toolIntent(t, tool({ fn: "my_tool", args: "{" }))).toEqual({
      label: { kind: "text", value: "my_tool" },
    });
  });

  it("derives ordered meta badges", () => {
    expect(
      toolMetaItems(t, tool({ added: 3, removed: 2, hits: 7, exitCode: 1, status: "running" })),
    ).toEqual([
      // No added/removed here: a diffstat is one fact, and it is rendered by the
      // atom the diff views use rather than by two chips of its own.
      { id: "hits", label: "7 matches", tone: "muted" },
      { id: "exit", label: "exit 1", tone: "negative" },
      { id: "live", label: "live", tone: "muted" },
    ]);
  });

  it("reports a plan's progress and a partial read's span as notation", () => {
    expect(toolMetaItems(t, tool({ progress: { done: 3, total: 7 } }))).toEqual([
      { id: "progress", label: "3/7", tone: "muted" },
    ]);
    expect(toolMetaItems(t, tool({ range: { start: 40, end: 80 }, lines: 900 }))).toEqual([
      { id: "range", label: "L40-80", tone: "muted" },
      { id: "lines", label: "900 lines", tone: "muted" },
    ]);
  });

  it("reports a diffstat only when it has something to say", () => {
    expect(toolDiffStat(tool({ added: 3, removed: 2 }))).toEqual({ added: 3, removed: 2 });
    expect(toolDiffStat(tool({ added: 4 }))).toEqual({ added: 4, removed: 0 });
    // A dash holds a column in the diff views; on a transcript row it is a mark
    // the reader has to stop and interpret.
    expect(toolDiffStat(tool({ added: 0, removed: 0 }))).toBeUndefined();
    expect(toolDiffStat(tool({}))).toBeUndefined();
  });

  // The runtime measures the call; a sub-second read reporting "0.1s" is noise on
  // every row, so the number only appears once it can explain a wait.
  it("reports a measured duration, and only once it is worth reading", () => {
    expect(toolMetaItems(t, tool({ durationMs: 4200 })).map((item) => item.id)).toEqual([
      "duration",
    ]);
    expect(toolMetaItems(t, tool({ durationMs: 120 }))).toEqual([]);
    expect(toolMetaItems(t, tool({}))).toEqual([]);
  });

  // The runtime's own safety class, not a list of tool names kept here: a tool
  // renamed on the backend used to silently change weight in the transcript.
  it("takes read-only from the runtime's safety class", () => {
    expect(isReadOnlyTool(tool({ name: "read", safetyClass: "safe" }))).toBe(true);
    expect(isReadOnlyTool(tool({ name: "edit", safetyClass: "write" }))).toBe(false);
    // Unclassified (an MCP tool the runtime has no class for) is not a read.
    expect(isReadOnlyTool(tool({ name: "acme_do_thing" }))).toBe(false);
  });

  it("summarizes grouped tools by display bucket", () => {
    const tools = [
      tool({ id: "read", name: "read" }),
      tool({ id: "grep", name: "grep" }),
      tool({ id: "glob", name: "glob" }),
      tool({ id: "lsp", name: "lsp" }),
    ];
    expect(summarizeToolGroup(t, tools)).toBe("1 read · 2 search · 1 lookup");
  });

  it("marks groups needing attention only while running or failed", () => {
    expect(toolGroupNeedsAttention([tool({ status: "ok" })])).toBe(false);
    expect(toolGroupNeedsAttention([tool({ status: "running" })])).toBe(true);
    expect(toolGroupNeedsAttention([tool({ status: "err" })])).toBe(true);
  });
});
