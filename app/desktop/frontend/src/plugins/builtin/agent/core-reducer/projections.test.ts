// Locks the per-tool display projections against the RUNTIME's actual wire
// shapes (lynx/lyra tool implementations), not just the §4.4.2 conventions:
// shell returns {stdout, stderr, exit_code}, grep one of matches/files/counts,
// glob {paths}, edit/write {changes:[{path,status}]} (no per-file diff rows),
// and the specialised tools (lsp / lsp_diagnostics / skill / task / ask_user /
// shell_output / shell_kill) label by name.

import type { ToolInvocation } from "@/rpc";
import { describe, expect, it } from "vitest";
import { argsText, toolFields, toolLabel } from "./projections";

const tool = (name: string, args: Record<string, unknown>, result?: unknown): ToolInvocation =>
  ({ name, arguments: args, result }) as ToolInvocation;

describe("toolLabel — name-keyed specialised tools", () => {
  it("lsp position operations label file:line:character", () => {
    expect(
      toolLabel(
        tool("lsp", { operation: "definition", file_path: "main.go", line: 42, character: 3 }),
      ),
    ).toBe("main.go:42:3");
    expect(
      toolLabel(tool("lsp", { operation: "hover", file_path: "a.ts", line: 1, character: 5 })),
    ).toBe("a.ts:1:5");
  });

  it("lsp file/query operations label by their key argument", () => {
    expect(toolLabel(tool("lsp", { operation: "document_symbols", file_path: "main.go" }))).toBe(
      "main.go",
    );
    expect(toolLabel(tool("lsp", { operation: "workspace_symbols", query: "ReadTool" }))).toBe(
      "ReadTool",
    );
    expect(toolLabel(tool("lsp_diagnostics", { file_path: "main.go" }))).toBe("main.go");
  });

  it("skill labels op + name; ask_user labels the first question", () => {
    expect(toolLabel(tool("skill", { op: "load", name: "review" }))).toBe("load review");
    expect(toolLabel(tool("skill", { op: "list" }))).toBe("list");
    expect(
      toolLabel(tool("ask_user", { questions: [{ question: "Deploy now?\ndetails…" }] })),
    ).toBe("Deploy now?");
  });

  it("task (subagent category) labels the prompt's first line", () => {
    expect(toolLabel(tool("task", { prompt: "Investigate flaky test\nthen report" }))).toBe(
      "Investigate flaky test",
    );
  });

  it("background-shell tools: run_in_background labels the command, pollers the shell id", () => {
    expect(toolLabel(tool("run_in_background", { command: "npm run dev" }))).toBe("npm run dev");
    expect(toolLabel(tool("shell_output", { shell_id: "bg_1" }))).toBe("bg_1");
    expect(toolLabel(tool("shell_kill", { shell_id: "bg_2" }))).toBe("bg_2");
  });
});

describe("toolFields — runtime wire shapes", () => {
  it("shell: merges stdout+stderr and reads snake_case exit_code", () => {
    const f = toolFields(
      tool("shell", { command: "go test" }, { stdout: "ok", stderr: "warn", exit_code: 1 }),
    );
    expect(f.result).toBe("ok\nwarn");
    expect(f.exitCode).toBe(1);
  });

  it("shell: still honors the §4.4.2 {output, exitCode} convention", () => {
    const f = toolFields(tool("shell", {}, { output: "done", exitCode: 0 }));
    expect(f.result).toBe("done");
    expect(f.exitCode).toBe(0);
  });

  it("run_in_background: passes the plain-string start ack through", () => {
    const f = toolFields(
      tool("run_in_background", { command: "npm run dev" }, "Started background shell bg_1."),
    );
    expect(f.result).toBe("Started background shell bg_1.");
    expect(f.exitCode).toBeUndefined();
  });

  it("grep: hits from whichever of matches/files/counts is populated", () => {
    expect(toolFields(tool("grep", {}, { matches: [{}, {}] })).hits).toBe(2);
    expect(toolFields(tool("grep", {}, { files: ["a", "b", "c"] })).hits).toBe(3);
    expect(toolFields(tool("glob", {}, { paths: ["x"] })).hits).toBe(1);
  });

  it("edit: no fabricated ±0 counts when the result has no diff rows", () => {
    // The runtime's ACTUAL write/edit shape (tooldisplay.go): file entries with
    // status but no per-file `diff` — must NOT render "+0 −0".
    const f = toolFields(
      tool("edit", { file_path: "a.go" }, { changes: [{ path: "a.go", status: "modified" }] }),
    );
    expect(f.added).toBeUndefined();
    expect(f.removed).toBeUndefined();
    // A result with no `changes` key at all stays {} too.
    const g = toolFields(tool("write", { file_path: "b.go" }, { bytes_written: 12 }));
    expect(g.added).toBeUndefined();
    expect(g.removed).toBeUndefined();
  });

  it("read: passes the content through as the result body", () => {
    expect(toolFields(tool("read", { path: "a.go" }, { content: "package main" })).result).toBe(
      "package main",
    );
  });
});

describe("argsText — fn-baked tools suppress the raw JSON echo", () => {
  it("name-labelled tools return empty args text", () => {
    expect(
      argsText(tool("lsp", { operation: "definition", file_path: "a.go", line: 1, character: 1 })),
    ).toBe("");
    expect(argsText(tool("skill", { op: "list" }))).toBe("");
  });

  it("generic (MCP) tools keep the JSON args fallback", () => {
    expect(argsText(tool("linear.create_issue", { title: "t" }))).toContain('"title"');
  });
});
