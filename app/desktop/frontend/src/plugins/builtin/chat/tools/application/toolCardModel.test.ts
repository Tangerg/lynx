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
      shell: "flagged",
      tone: "warning",
    });
    expect(model.intent.label).toBeTruthy();
    expect(Array.isArray(model.metaItems)).toBe(true);
  });

  // The read/produce split is what keeps a turn from reading as one grey stack, and
  // it is a table in the presentation ring rather than a condition in the card — so
  // this is where it has to be pinned.
  it("gives a glance a line, a product a card, and trouble an edge", () => {
    const read = { name: "read", safetyClass: "safe" } as const;
    expect(toolCardModel(t, tool({ ...read, status: "ok" })).shell).toBe("line");
    expect(toolCardModel(t, tool({ name: "lsp", safetyClass: "safe", status: "ok" })).shell).toBe(
      "line",
    );
    expect(toolCardModel(t, tool({ name: "shell", status: "ok" })).shell).toBe("card");
    expect(
      toolCardModel(t, tool({ name: "edit", safetyClass: "write", status: "running" })).shell,
    ).toBe("card");
    // A read that FAILED is not a glance any more.
    expect(toolCardModel(t, tool({ ...read, status: "err" })).shell).toBe("flagged");
  });

  it("tells a refused call apart from a finished one", () => {
    const denied = toolCardModel(t, tool({ status: "denied" }));
    expect(denied).toMatchObject({ denied: true, shell: "flagged", tone: "warning" });
    expect(toolCardModel(t, tool({ status: "ok" })).denied).toBe(false);
  });

  // The tick is a fallback, not a fixture: where a call reported counts, those ARE
  // the verdict, and a tick after them is one more identical glyph in a column.
  it("only marks a settled call that had nothing to report", () => {
    expect(toolCardModel(t, tool({ status: "ok" })).showSettledMark).toBe(true);
    expect(toolCardModel(t, tool({ status: "ok", hits: 9 })).showSettledMark).toBe(false);
    expect(toolCardModel(t, tool({ status: "running" })).showSettledMark).toBe(false);
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
