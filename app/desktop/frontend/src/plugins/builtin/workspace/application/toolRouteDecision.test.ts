import { describe, expect, it } from "vitest";
import type { ToolCall } from "@/protocol/run/viewState";
import { decideWorkspaceToolRoute, hasWorkspaceToolView } from "./toolRouteDecision";

const tool = (over: Partial<ToolCall> & Pick<ToolCall, "name">): ToolCall => ({
  id: "t1",
  fn: "",
  args: "",
  status: "ok",
  ...over,
});

describe("decideWorkspaceToolRoute", () => {
  it("routes command tools to the terminal view", () => {
    expect(decideWorkspaceToolRoute(tool({ name: "shell", fn: "ls -la" }))).toEqual({
      view: { id: "terminal", title: "workspace.view.title.terminal", icon: "terminal" },
    });
  });

  it("routes edit tools to the diff view and exposes the focused file", () => {
    expect(decideWorkspaceToolRoute(tool({ name: "edit", fn: "src/app.ts" }))).toEqual({
      view: { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      activeFile: "src/app.ts",
    });
  });

  it("does not treat multi-file labels as file paths", () => {
    expect(decideWorkspaceToolRoute(tool({ name: "write", fn: "3 files" }))).toEqual({
      view: { id: "diff", title: "workspace.view.title.diff", icon: "diff" },
      activeFile: undefined,
    });
  });

  it("does not route inline-only tool categories", () => {
    const search = tool({ name: "grep", fn: "needle" });

    expect(hasWorkspaceToolView(search)).toBe(false);
    expect(decideWorkspaceToolRoute(search)).toBeNull();
  });
});
