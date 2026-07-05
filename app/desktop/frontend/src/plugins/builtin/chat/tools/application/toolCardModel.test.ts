import { describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";
import {
  toolCardActions,
  toolCardModel,
  toolCardViewOpener,
  visibleToolMetaItems,
} from "./toolCardModel";

const tool = (overrides: Partial<ToolCall> = {}): ToolCall => ({
  id: "tool-1",
  name: "shell",
  fn: "shell",
  args: "go test ./...",
  status: "ok",
  ...overrides,
});

describe("toolCardModel", () => {
  it("lets an error message own the collapsed detail line", () => {
    expect(
      toolCardModel(
        tool({
          status: "err",
          error: "permission denied",
          args: '{"cmd":"rm"}',
        }),
      ),
    ).toMatchObject({
      isError: true,
      detail: "permission denied",
    });
  });

  it("projects lifecycle flags and presentation data", () => {
    const model = toolCardModel(tool({ status: "requires-action" }));

    expect(model).toMatchObject({
      running: false,
      isError: false,
      needsAction: true,
    });
    expect(model.intent.label).toBeTruthy();
    expect(Array.isArray(model.metaItems)).toBe(true);
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
