import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";
import {
  toolCardActions,
  toolCardModel,
  toolCardViewOpener,
  visibleToolMetaItems,
} from "./toolCardModel";

const tool = ({ runId = "run_1", ...overrides }: Partial<ToolCall> = {}): ToolCall => ({
  id: "tool-1",
  runId,
  name: "shell",
  fn: "shell",
  args: "go test ./...",
  status: "ok",
  safetyClass: "exec",
  ...overrides,
});

describe("toolCardModel", () => {
  it("lets an error message own the collapsed detail line", () => {
    expect(
      toolCardModel(
        t,
        tool({
          status: "err",
          error: "permission denied",
          args: '{"cmd":"rm"}',
        }),
      ),
    ).toMatchObject({
      isError: true,
      detail: { kind: "text", value: "permission denied" },
    });
  });

  it("projects lifecycle flags and presentation data", () => {
    const model = toolCardModel(t, tool({ status: "requires-action" }));

    expect(model).toMatchObject({
      running: false,
      isError: false,
      shell: "line",
      tone: "neutral",
    });
    expect(model.intent.label).toBeTruthy();
    expect(Array.isArray(model.metaItems)).toBe(true);
  });

  // Codex keeps the invocation itself in the work narrative: read, write,
  // running, failed and refused calls all use the same transparent row. The
  // material result (terminal output or diff) earns a surface only after the row
  // is opened; lifecycle truth stays inline instead of turning the row into a
  // status card.
  it("keeps every invocation on the Codex narrative line", () => {
    const cases: Array<Partial<ToolCall>> = [
      { name: "read", safetyClass: "safe", status: "ok" },
      { name: "shell", safetyClass: "exec", status: "running" },
      { name: "apply_patch", safetyClass: "write", status: "ok" },
      { name: "apply_patch", safetyClass: "write", status: "err" },
      { name: "apply_patch", safetyClass: "write", status: "denied" },
    ];

    expect(cases.map((entry) => toolCardModel(t, tool(entry)).shell)).toEqual([
      "line",
      "line",
      "line",
      "line",
      "line",
    ]);
  });

  it("tells a refused call apart from a finished one", () => {
    const denied = toolCardModel(t, tool({ status: "denied" }));
    expect(denied).toMatchObject({ denied: true, shell: "line", tone: "neutral" });
    expect(toolCardModel(t, tool({ status: "ok" })).denied).toBe(false);
  });
});

describe("toolCardActions", () => {
  it("keeps actions with no predicate or a matching predicate", () => {
    const actions: ToolActionSpec[] = [
      { id: "always", icon: "copy", title: "Always", run: () => undefined },
      {
        id: "shell",
        icon: "terminal",
        title: "Shell",
        predicate: (candidate) => candidate.name === "shell",
        run: () => undefined,
      },
      {
        id: "read",
        icon: "file",
        title: "Read",
        predicate: (candidate) => candidate.name === "read",
        run: () => undefined,
      },
    ];

    expect(toolCardActions(tool({ name: "shell" }), actions).map((action) => action.id)).toEqual([
      "always",
      "shell",
    ]);
  });
});

describe("toolCardViewOpener", () => {
  it("selects the first opener whose predicate matches the tool", () => {
    const openers: ToolViewOpenerSpec[] = [
      { id: "read", predicate: (candidate) => candidate.name === "read", open: () => undefined },
      { id: "shell", predicate: (candidate) => candidate.name === "shell", open: () => undefined },
    ];

    expect(toolCardViewOpener(tool({ name: "shell" }), openers)?.id).toBe("shell");
  });
});

describe("visibleToolMetaItems", () => {
  it("hides the live text when the running dot already carries that state", () => {
    const items = [
      { id: "live", label: "live", tone: "muted" },
      { id: "hits", label: "3 hits", tone: "muted" },
    ] as const;

    expect(visibleToolMetaItems(items, true)).toEqual([items[1]]);
    expect(visibleToolMetaItems(items, false)).toEqual([...items]);
  });
});
