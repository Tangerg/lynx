// Locks the per-tool display projections against the RUNTIME's actual wire
// shapes (lynx/lyra tool implementations), not just the §4.4.2 conventions:
// bash returns {stdout, stderr, exit_code}, grep one of matches/files/counts,
// glob {paths}, edit/write {replacements}/{bytes_written} (no diff rows),
// and the specialised tools (lsp_* / skill / task / ask_user) label by name.

import type { ToolInvocation } from "@/rpc";
import { describe, expect, it } from "vitest";
import { argsText, toolFields, toolLabel } from "./projections";

const tool = (name: string, args: Record<string, unknown>, result?: unknown): ToolInvocation =>
  ({ name, arguments: args, result }) as ToolInvocation;

describe("toolLabel — name-keyed specialised tools", () => {
  it("lsp position tools label file:line:column", () => {
    expect(toolLabel(tool("lsp_definition", { file: "main.go", line: 42, column: 3 }))).toBe(
      "main.go:42:3",
    );
    expect(toolLabel(tool("lsp_hover", { file: "a.ts", line: 1, column: 5 }))).toBe("a.ts:1:5");
  });

  it("lsp file/query tools label by their key argument", () => {
    expect(toolLabel(tool("lsp_diagnostics", { file: "main.go" }))).toBe("main.go");
    expect(toolLabel(tool("lsp_workspace_symbols", { query: "ReadTool" }))).toBe("ReadTool");
  });

  it("skill labels op + name; ask_user labels the question", () => {
    expect(toolLabel(tool("skill", { op: "load", name: "review" }))).toBe("load review");
    expect(toolLabel(tool("skill", { op: "list" }))).toBe("list");
    expect(toolLabel(tool("ask_user", { question: "Deploy now?\ndetails…" }))).toBe("Deploy now?");
  });

  it("task (subagent category) labels the prompt's first line", () => {
    expect(toolLabel(tool("task", { prompt: "Investigate flaky test\nthen report" }))).toBe(
      "Investigate flaky test",
    );
  });

  it("background-shell tools: run_in_background labels the command, pollers the shell id", () => {
    expect(toolLabel(tool("run_in_background", { command: "npm run dev" }))).toBe("npm run dev");
    expect(toolLabel(tool("bash_output", { shell_id: "bg_1" }))).toBe("bg_1");
    expect(toolLabel(tool("kill_shell", { shell_id: "bg_2" }))).toBe("bg_2");
  });
});

describe("toolFields — runtime wire shapes", () => {
  it("bash: merges stdout+stderr and reads snake_case exit_code", () => {
    const f = toolFields(
      tool("bash", { command: "go test" }, { stdout: "ok", stderr: "warn", exit_code: 1 }),
    );
    expect(f.result).toBe("ok\nwarn");
    expect(f.exitCode).toBe(1);
  });

  it("bash: still honors the §4.4.2 {output, exitCode} convention", () => {
    const f = toolFields(tool("bash", {}, { output: "done", exitCode: 0 }));
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
    const f = toolFields(tool("edit", { path: "a.go" }, { replacements: 2 }));
    expect(f.added).toBeUndefined();
    expect(f.removed).toBeUndefined();
  });

  it("read: passes the content through as the result body", () => {
    expect(toolFields(tool("read", { path: "a.go" }, { content: "package main" })).result).toBe(
      "package main",
    );
  });
});

describe("argsText — fn-baked tools suppress the raw JSON echo", () => {
  it("name-labelled tools return empty args text", () => {
    expect(argsText(tool("lsp_definition", { file: "a.go", line: 1, column: 1 }))).toBe("");
    expect(argsText(tool("skill", { op: "list" }))).toBe("");
  });

  it("generic (MCP) tools keep the JSON args fallback", () => {
    expect(argsText(tool("linear.create_issue", { title: "t" }))).toContain('"title"');
  });
});
