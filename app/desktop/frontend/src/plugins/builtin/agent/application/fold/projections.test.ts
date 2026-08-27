// Locks the per-tool display projections against the RUNTIME's actual wire
// shapes (scope/scopeapp tool implementations): the presenter projects shell to
// {output, exitCode}, grep and glob to {hits}, apply_patch to
// {changes:[{path,status}]}, and the name-keyed tools (lsp / the Skill family /
// ask_user / read_shell_output / stop_shell / the searches / the fetches) label by
// their own key argument.

import type { ToolInvocation } from "@/rpc";
import { describe, expect, it } from "vitest";
import { argsText, toolFields, toolLabel } from "./projections";

const tool = (name: string, args: Record<string, unknown>, result?: unknown): ToolInvocation =>
  ({ name, arguments: args, result }) as ToolInvocation;

describe("toolLabel — name-keyed specialised tools", () => {
  it("lsp position operations label path:line:character", () => {
    expect(
      toolLabel(tool("lsp", { operation: "definition", path: "main.go", line: 42, character: 3 })),
    ).toBe("main.go:42:3");
    expect(
      toolLabel(tool("lsp", { operation: "hover", path: "a.ts", line: 1, character: 5 })),
    ).toBe("a.ts:1:5");
  });

  it("lsp file/query operations label by their key argument", () => {
    expect(toolLabel(tool("lsp", { operation: "document_symbols", path: "main.go" }))).toBe(
      "main.go",
    );
    expect(toolLabel(tool("lsp", { operation: "workspace_symbols", query: "ReadTool" }))).toBe(
      "ReadTool",
    );
    // Diagnostics is one of this tool's operations, not a tool of its own.
    expect(toolLabel(tool("lsp", { operation: "diagnostics", path: "main.go" }))).toBe("main.go");
  });

  it("the Skill family labels by name; ask_user labels the first question", () => {
    expect(toolLabel(tool("load_skill", { name: "review" }))).toBe("review");
    expect(toolLabel(tool("propose_skill", { name: "review-go-api" }))).toBe("review-go-api");
    expect(toolLabel(tool("read_skill_resource", { name: "review", path: "checklist.md" }))).toBe(
      "review/checklist.md",
    );
    expect(
      toolLabel(tool("ask_user", { questions: [{ question: "Deploy now?\ndetails…" }] })),
    ).toBe("Deploy now?");
  });

  it("delegate_task (subagent category) labels its summary", () => {
    expect(
      toolLabel(tool("delegate_task", { summary: "Audit the retry path", instructions: "…" })),
    ).toBe("Audit the retry path");
  });

  it("the searches and fetches label by query and url", () => {
    expect(toolLabel(tool("search_memory", { query: "deploy runbook" }))).toBe("deploy runbook");
    expect(toolLabel(tool("search_conversations", { query: "flaky test" }))).toBe("flaky test");
    expect(toolLabel(tool("search_tools", { query: "screenshot" }))).toBe("screenshot");
    expect(toolLabel(tool("web_fetch", { url: "https://go.dev" }))).toBe("https://go.dev");
    expect(toolLabel(tool("http_request", { url: "https://api.local/health" }))).toBe(
      "https://api.local/health",
    );
  });

  // The runtime requires a human action phrase on every shell call, so the row's
  // title is that phrase and the command line rides beside it as the detail.
  it("shell labels its description, and the shell pollers their shell id", () => {
    expect(
      toolLabel(tool("shell", { command: "npm run dev", description: "Start the dev server" })),
    ).toBe("Start the dev server");
    expect(toolLabel(tool("read_shell_output", { shell_id: "bg_1" }))).toBe("bg_1");
    expect(toolLabel(tool("stop_shell", { shell_id: "bg_2" }))).toBe("bg_2");
  });
});

describe("toolFields — runtime wire shapes", () => {
  // The command reaches the view as its own field: the row's title is the human
  // description, so without this the one line a reader verifies is nowhere.
  it("shell: reads the projected {output, exitCode} and carries the command", () => {
    const f = toolFields(
      tool(
        "shell",
        { command: "go test", description: "Run the tests" },
        { output: "ok\nwarn", exitCode: 1 },
      ),
    );
    expect(f.result).toBe("ok\nwarn");
    expect(f.exitCode).toBe(1);
    expect(f.command).toBe("go test");
  });

  it("shell: a backgrounded call's plain-string ack passes through with no exit code", () => {
    const f = toolFields(
      tool(
        "shell",
        { command: "npm run dev", run_in_background: true },
        "Started background shell bg_1.",
      ),
    );
    expect(f.result).toBe("Started background shell bg_1.");
    expect(f.exitCode).toBeUndefined();
  });

  it("grep: hits come from the runtime's single projected envelope", () => {
    expect(toolFields(tool("grep", {}, { hits: [{}, {}] })).hits).toBe(2);
    expect(toolFields(tool("grep", {}, { hits: ["a", "b", "c"] })).hits).toBe(3);
    expect(toolFields(tool("glob", {}, { hits: [{ path: "x" }] })).hits).toBe(1);
  });

  it("apply_patch: does not fabricate line counts absent from its receipt", () => {
    // The Runtime's actual patch shape contains path/status facts but no
    // per-file diff rows, so the transcript must not render "+0 −0".
    const f = toolFields(
      tool("apply_patch", { patch: "…" }, { changes: [{ path: "a.go", status: "modified" }] }),
    );
    expect(f.added).toBeUndefined();
    expect(f.removed).toBeUndefined();
    // An unrecognised result still carries no invented line counts.
    const g = toolFields(tool("apply_patch", { patch: "…" }, { files: [] }));
    expect(g.added).toBeUndefined();
    expect(g.removed).toBeUndefined();
  });

  it("apply_patch: carries the authoritative call-scoped change receipt", () => {
    const f = toolFields(
      tool(
        "apply_patch",
        { patch: "*** Begin Patch\n…" },
        {
          changes: [
            { path: "src/new.ts", status: "added" },
            { path: "src/current.ts", status: "moved", from: "src/old.ts" },
          ],
        },
      ),
    );

    expect(JSON.parse(f.result ?? "null")).toEqual({
      changes: [
        { path: "src/new.ts", status: "added" },
        { path: "src/current.ts", status: "moved", from: "src/old.ts" },
      ],
    });
  });

  it("read: passes the content through and reports how long the file is", () => {
    expect(
      toolFields(tool("read", { path: "a.go" }, { content: "package main", total_lines: 412 })),
    ).toMatchObject({ result: "package main", lines: 412 });
    // A runtime that reports no length gets no chip, rather than a "0 lines" one.
    expect(toolFields(tool("read", { path: "a.go" }, { content: "x" })).lines).toBeUndefined();
  });
});

describe("argsText — fn-baked tools suppress the raw JSON echo", () => {
  it("name-labelled tools return empty args text", () => {
    expect(
      argsText(tool("lsp", { operation: "definition", path: "a.go", line: 1, character: 1 })),
    ).toBe("");
    expect(argsText(tool("load_skill", { name: "review" }))).toBe("");
  });

  it("generic (MCP) tools keep the JSON args fallback", () => {
    // MCP model-facing names are sanitize("<server>_<tool>") — underscores, no dots.
    expect(argsText(tool("linear_create_issue", { title: "t" }))).toContain('"title"');
  });
});
